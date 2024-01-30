package main

import (
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"sync/atomic"
	"syscall"
)

var debugMode bool = true // set to false to disable debug messages

const (
	maxDepth   = 100000 // Maximum depth limit to prevent stack overflow
	safeDepth  = 5000   // Safe depth limit to resume job execution
	delayUnit  = 100    // Base unit of delay in milliseconds
	bufferSize = 10000
)

// JobFunc is an interface that requires a specific signature for job functions.
type JobFunc interface {
	Invoke(args ...interface{}) JobResult
}

// JobFuncImpl wraps a function to conform to the JobFunc interface.
type JobFuncImpl struct {
	fn interface{}
}

func (jfi JobFuncImpl) Invoke(args ...interface{}) JobResult {
	reflectFunc := reflect.ValueOf(jfi.fn)
	if reflectFunc.Kind() != reflect.Func {
		return JobResult{Error: fmt.Errorf("provided job is not a function")}
	}

	reflectFuncType := reflectFunc.Type()
	if reflectFuncType.NumIn() != len(args) {
		return JobResult{Error: fmt.Errorf("expected %d arguments, got %d", reflectFuncType.NumIn(), len(args))}
	}

	var reflectArgs []reflect.Value
	for i, arg := range args {
		argType := reflectFuncType.In(i)

		// If the function expects an interface{}, allow any type.
		if argType.Kind() == reflect.Interface {
			reflectArgs = append(reflectArgs, reflect.ValueOf(arg))
		} else {
			// Retrieve the argument's value considering it might be a pointer.
			argValue := reflect.ValueOf(arg)
			if argValue.Kind() == reflect.Ptr {
				argValue = argValue.Elem()
			}

			// Perform type checking and append the argument.
			if argValue.Type() != argType {
				return JobResult{Error: fmt.Errorf("argument %d must be of type %s, got %s", i, argType, argValue.Type())}
			}
			reflectArgs = append(reflectArgs, argValue)
		}
	}

	results := reflectFunc.Call(reflectArgs)
	if len(results) > reflectFuncType.NumOut() {
		return JobResult{Error: fmt.Errorf("function returned more values than expected")}
	}

	var result interface{}
	var err error

	if len(results) > 0 {
		// Handle the first returned value (result)
		result = results[0].Interface()

		// Handle the second returned value (error), if any
		if len(results) > 1 {
			errValue := results[1]
			if !errValue.IsNil() {
				err, _ = errValue.Interface().(error)
			}
		}
	}

	return JobResult{Result: result, Error: err}
}

// JobResult represents the result of a job.
type JobResult struct {
	Result  interface{}
	JobName string
	Client  string
	Error   error
}

// type JobManager struct {
// 	jobs  map[string]JobFunc
// 	depth int   // Track the depth of job calls
// 	queue []Job // Queue for jobs to be processed
// }

type JobManager struct {
	jobs          map[string]JobFunc
	depth         int32 // Use int32 for atomic operations
	queue         []Job
	lock          sync.Mutex     // Protects the job queue
	wg            sync.WaitGroup // Waits for all jobs to complete
	results       chan JobResult // Channel for job results
	subscriptions map[string][]string
	jobCh         map[string]chan interface{}
	jobAssignment map[string][]string
}

type Job struct {
	Name   string
	Client string
	Args   []interface{}
}

func NewJobManager() *JobManager {
	return &JobManager{
		jobs:          make(map[string]JobFunc),
		subscriptions: make(map[string][]string),
		jobCh:         make(map[string]chan interface{}),
		results:       make(chan JobResult, bufferSize), // Utilize bufferSize here
		jobAssignment: make(map[string][]string),
	}
}

func (jm *JobManager) RegisterJob(name string, job interface{}) {
	jm.jobs[name] = JobFuncImpl{fn: job}
	if _, exists := jm.jobCh[name]; !exists {
		jm.jobCh[name] = make(chan interface{}, bufferSize) // Ensure a channel is created for each job
	}
}

func (jm *JobManager) ProcessJobs() {
	for len(jm.queue) > 0 {
		job := jm.queue[0]
		jm.queue = jm.queue[1:]

		result := jm.jobs[job.Name].Invoke(job.Args...)
		handleJobResult(job.Name, job.Args, result)
	}
}

// handleJobResult prints the result of a job or any error that occurred.
func handleJobResult(jobName string, args interface{}, result JobResult) {
	if debugMode {
		fmt.Printf("Job '%s' was called with input: %+v\n", jobName, args)
		if result.Error != nil {
			fmt.Printf("Error in job '%s': %v\n", jobName, result.Error)
		} else {
			fmt.Printf("Result of job '%s': %+v\n", jobName, result.Result)
		}
	} else {
		if result.Error != nil {
			fmt.Printf("Error in job %s: %v\n", jobName, result.Error)
			return
		}
		fmt.Printf("Result of job %s: %+v\n", jobName, result.Result)
	}
}

func gracefulTermination() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	fmt.Println("Program exiting gracefully")
}

// jobA processes an integer input by adding 1.
func jobA(arg interface{}) (interface{}, error) {
	input, ok := arg.(int)
	if !ok {
		return nil, fmt.Errorf("jobA expects an int argument")
	}
	return input + 1, nil
}

// jobB processes an integer input by multiplying it by 2.
func jobB(arg interface{}) (interface{}, error) {
	input, ok := arg.(int)
	if !ok {
		return nil, fmt.Errorf("jobB expects an int argument")
	}
	return input * 2, nil
}

// job2 processes an integer input by multiplying it by 3.
func job2(arg interface{}) (interface{}, error) {
	input, ok := arg.(int)
	if !ok {
		return nil, fmt.Errorf("job2 expects an int argument")
	}
	return input * 3, nil
}

// job3 processes an integer input by subtracting 2.
func job3(arg interface{}) (interface{}, error) {
	input, ok := arg.(int)
	if !ok {
		return nil, fmt.Errorf("job3 expects an int argument")
	}
	return input - 2, nil
}

// jobI processes an integer input and adds a specific value.
func jobI(arg interface{}) (interface{}, error) {
	input, ok := arg.(int)
	if !ok {
		return nil, fmt.Errorf("jobI expects an int argument")
	}
	return input + 10, nil // Modify as needed
}

// jobJ processes an integer input, it depends on jobI.
func jobJ(arg interface{}) (interface{}, error) {
	input, ok := arg.(int)
	if !ok {
		return nil, fmt.Errorf("jobJ expects an int argument")
	}
	// Assume jobJ does something with the output of jobI
	return input * 2, nil // Modify as needed
}

// Subscribe allows a job to subscribe to another job's results
func (jm *JobManager) Subscribe(publisher, subscriber string) {
	jm.subscriptions[publisher] = append(jm.subscriptions[publisher], subscriber)
	if _, ok := jm.jobCh[subscriber]; !ok {
		jm.jobCh[subscriber] = make(chan interface{}, bufferSize) // Buffer size 1 for immediate value
	}
}

// Publish sends the result of a job to all subscribers
func (jm *JobManager) Publish(jobName string, result interface{}) {
	subscribers, ok := jm.subscriptions[jobName]
	if !ok {
		return // No subscribers for this job
	}
	for _, subscriberJobName := range subscribers {
		jobChannel, ok := jm.jobCh[subscriberJobName]
		if !ok {
			continue // Subscriber job channel not found
		}
		select {
		case jobChannel <- result:
		default:
			fmt.Printf("Warning: Job channel for %s is full. Result not sent.\n", subscriberJobName)
			// Handle the case when the channel is full (e.g., logging, alternative action)
		}
	}
}

// ProcessResults listens for job results and publishes them to subscribers
func (jm *JobManager) ProcessResults() {
	for jobResult := range jm.results {
		handleJobResult(jobResult.JobName, jobResult.Client, jobResult)
		jm.Publish(jobResult.JobName, jobResult.Result)
	}
}

func startJobs(jm *JobManager) {
	initialInput := 10
	for clientID, jobs := range jm.jobAssignment {
		for _, jobName := range jobs {
			go func(clientID, jobName string) {
				jm.StartJob(jobName, clientID, initialInput)
				jm.wg.Add(1) // Add 1 to the wait group for each job started
			}(clientID, jobName)
		}
	}
	jm.wg.Wait()
}

func setupJobManager() *JobManager {
	jm := NewJobManager()
	registerJobs(jm)
	defineJobAssignments(jm) // Predefine the job assignments
	setupSubscriptions(jm)   // Setup subscriptions
	return jm
}

func defineJobAssignments(jm *JobManager) {
	// Define job assignments for each client
	jm.jobAssignment["client1"] = []string{"jobI", "jobA"}
	jm.jobAssignment["client2"] = []string{"jobJ", "jobB"}
	jm.jobAssignment["client3"] = []string{"jobI", "jobJ", "jobA", "jobB"} // Example of overlapping jobs for a client
	jm.jobAssignment["client4"] = []string{"job2", "job3"}
}

func registerJobs(jm *JobManager) {
	jm.RegisterJob("jobA", jobA)
	jm.RegisterJob("jobB", jobB)
	jm.RegisterJob("job2", job2)
	jm.RegisterJob("job3", job3)
	jm.RegisterJob("jobI", jobI)
	jm.RegisterJob("jobJ", jobJ)
}

func setupSubscriptions(jm *JobManager) {
	// Define job subscriptions
	jm.Subscribe("jobI", "jobJ")
	jm.Subscribe("jobJ", "jobI")
}

func main() {
	jm := setupJobManager()
	setupSubscriptions(jm)
	go jm.ProcessResults()

	startJobs(jm)

	gracefulTermination()
}

func runJobSequence(jm *JobManager, client string, initialArg interface{}, jobNames ...string) {
	// Start all jobs concurrently
	for _, jobName := range jobNames {
		jm.StartJob(jobName, client, initialArg) // All jobs start with the same initial argument
	}

	// Process results as they come in
	go func() {
		for i := 0; i < len(jobNames); i++ {
			result := <-jm.results
			handleJobResult(result.JobName, result.Client, result)
		}
	}()
}

func (jm *JobManager) StartJob(jobName string, client string, initialArg interface{}) {
	jm.wg.Add(1)
	go func() {
		defer jm.wg.Done()
		defer atomic.AddInt32(&jm.depth, -1) // Ensure depth is decremented when job is done

		if jobChannel, ok := jm.jobCh[jobName]; ok {
			if initialArg != nil {
				jobChannel <- initialArg
			}

			for input, ok := <-jobChannel; ok; input, ok = <-jobChannel {
				result := jm.jobs[jobName].Invoke(input)
				if result.Error != nil {
					logError(result.Error)
				} else {
					result.JobName = jobName
					result.Client = client
					jm.results <- result
				}
			}
		} else {
			logError(fmt.Errorf("Job channel for %s is not created", jobName))
		}
	}()
}

func logError(err error) {
	// Log the error
	fmt.Printf("Error: %v\n", err)
}

// func setupJobManager() *JobManager {
// 	jm := NewJobManager()
// 	registerJobs(jm)
// 	return jm
// }

// func startJobs(jm *JobManager) {
// 	initialInput := 10
// 	for clientID, jobs := range jm.jobAssignment {
// 		for _, jobName := range jobs {
// 			jm.StartJob(jobName, clientID, initialInput)
// 		}
// 	}

// 	jm.wg.Wait()
// }

// func main() {
// 	jm := setupJobManager()

// 	// Setup subscriptions
// 	setupSubscriptions(jm)

// 	// Start listening for job results in a separate goroutine
// 	go jm.ProcessResults()

// 	// Define job assignments for each client
// 	jm.jobAssignment["client1"] = []string{"jobI", "jobA"}
// 	jm.jobAssignment["client2"] = []string{"jobJ", "jobB"}
// 	jm.jobAssignment["client3"] = []string{"jobI", "jobJ", "jobA", "jobB"} // Example of overlapping jobs for a client

// 	initialInput := 10

// 	for clientID, jobs := range jm.jobAssignment {
// 		for _, jobName := range jobs {
// 			go func(clientID, jobName string) {
// 				jm.StartJob(jobName, clientID, initialInput)
// 				jm.wg.Add(1) // Add 1 to the wait group for each job started
// 			}(clientID, jobName)
// 		}
// 	}

// 	// Wait for all jobs to complete
// 	jm.wg.Wait()

// 	// Handle graceful termination
// 	gracefulTermination()
// }

// func setupJobManager() *JobManager {
// 	jm := NewJobManager()

// 	// Register jobs
// 	jm.RegisterJob("jobA", jobA)
// 	jm.RegisterJob("jobB", jobB)
// 	jm.RegisterJob("job2", job2)
// 	jm.RegisterJob("job3", job3)
// 	jm.RegisterJob("jobI", jobI)
// 	jm.RegisterJob("jobJ", jobJ)

// 	return jm
// }

// NewJobManager creates a new JobManager.
// func NewJobManager() *JobManager {
// 	return &JobManager{
// 		jobs:          make(map[string]JobFunc),
// 		subscriptions: make(map[string][]string),
// 		jobCh:         make(map[string]chan interface{}),
// 	}
// }

// // RegisterJob registers a job with the given name and function.
// func (jm *JobManager) RegisterJob(name string, job interface{}) {
// 	jm.jobs[name] = JobFuncImpl{fn: job}
// }

// func (jm *JobManager) RegisterJob(name string, job interface{}) {
// 	jm.jobs[name] = JobFuncImpl{fn: job}
// 	jm.jobCh[name] = make(chan interface{}, bufferSize) // Ensure a channel is created for each job
// }

// func startJobs(jm *JobManager) {
// 	initialInput := 10

// 	for clientID, jobs := range jm.jobAssignment {
// 		for _, jobName := range jobs {
// 			go func(clientID, jobName string) {
// 				jm.StartJob(jobName, clientID, initialInput)
// 				jm.wg.Add(1) // Add 1 to the wait group for each job started
// 			}(clientID, jobName)
// 		}
// 	}

// 	jm.wg.Wait()
// }

// func (jm *JobManager) StartJob(jobName string, client string, initialArg interface{}) {
// 	jm.wg.Add(1)
// 	go func() {
// 		defer jm.wg.Done()
// 		defer atomic.AddInt32(&jm.depth, -1) // Ensure depth is decremented when job is done

// 		// Check depth and apply delay if needed
// 		depth := atomic.AddInt32(&jm.depth, 1)
// 		if depth > maxDepth {
// 			fmt.Printf("Error: Job depth exceeded maximum limit (%d).\n", maxDepth)
// 			return
// 		}
// 		if depth > safeDepth {
// 			delayFactor := depth - safeDepth
// 			delay := math.Pow(2, float64(delayFactor-1)) * float64(delayUnit)
// 			fmt.Printf("Info: Job depth is high (%d). Execution is paused for %v milliseconds to manage load.\n", depth, delay)
// 			time.Sleep(time.Duration(delay) * time.Millisecond)
// 		}

// 		// Listening for input for the job
// 		jobChannel, ok := jm.jobCh[jobName]
// 		if !ok {
// 			fmt.Printf("Error: Job channel for %s is not created.\n", jobName)
// 			return
// 		}
// 		for input, ok := <-jobChannel; ok; input, ok = <-jobChannel {
// 			result := jm.jobs[jobName].Invoke(input)
// 			result.JobName = jobName
// 			result.Client = client
// 			jm.results <- result
// 		}
// 	}()

// 	// Sending the initial argument to the job
// 	if initialArg != nil {
// 		if jobChannel, ok := jm.jobCh[jobName]; ok {
// 			jobChannel <- initialArg
// 		} else {
// 			fmt.Printf("Error: Job channel for %s is not created.\n", jobName)
// 		}
// 	}
// }

// func main() {
// 	jm := setupJobManager()

// 	// Setup subscriptions
// 	setupSubscriptions(jm)

// 	// Start listening for job results in a separate goroutine
// 	go jm.ProcessResults()

// 	// Define client IDs
// 	clientIDs := []string{"client1", "client2", "client3"} // ปรับแต่งตามความต้องการ

// 	// Start jobs concurrently
// 	initialInput := 10
// 	jobsToStart := []string{"jobI", "jobJ", "jobA", "jobB"} // Add other jobs as needed

// 	for _, clientID := range clientIDs {
// 		for _, jobName := range jobsToStart {
// 			go func(clientID, jobName string) {
// 				jm.StartJob(jobName, clientID, initialInput)
// 			}(clientID, jobName)
// 		}
// 	}

// 	// Wait for all jobs to complete
// 	jm.wg.Wait()

// 	// Handle graceful termination
// 	gracefulTermination()
// }

// func main() {
// 	jm := setupJobManager()

// 	// Setup subscriptions
// 	setupSubscriptions(jm)

// 	// Start the initial job with a specific argument
// 	initialInput := 10
// 	jm.StartJob("jobI", "client1", initialInput)
// 	jm.StartJob("jobJ", "client1", initialInput) // สมมติว่าคุณต้องการเริ่ม jobJ ด้วย
// 	jm.StartJob("jobA", "client1", initialInput) // เริ่ม jobA
// 	jm.StartJob("jobB", "client1", initialInput) // เริ่ม jobB
// 	// ... เริ่ม job อื่นๆ ตามที่คุณต้องการ ...

// 	// Start listening for job results
// 	go jm.ProcessResults()

// 	// Wait for all jobs to complete
// 	jm.wg.Wait()

// 	// Handle graceful termination
// 	gracefulTermination()
// }

// func (jm *JobManager) Publish(jobName string, result interface{}) {
// 	for _, subscriberJobName := range jm.subscriptions[jobName] {
// 		// Forward the result to all subscriber jobs
// 		jm.StartJob(subscriberJobName, "client1", result)
// 	}
// }

//	func (jm *JobManager) Subscribe(jobName string, subscriberJobName string) {
//		jm.subscriptions[jobName] = append(jm.subscriptions[jobName], subscriberJobName)
//	}
//

// Publish sends the result of a job to all subscribers
// func (jm *JobManager) Publish(jobName string, result interface{}) {
// 	for _, subscriberJobName := range jm.subscriptions[jobName] {
// 		jm.jobCh[subscriberJobName] <- result
// 	}
// }

// func (jm *JobManager) ProcessResults() {
// 	for jobResult := range jm.results {
// 		handleJobResult(jobResult.JobName, jobResult.Client, jobResult)
// 		jm.Publish(jobResult.JobName, jobResult.Result)
// 	}
// }

// func (jm *JobManager) ProcessResults() {
// 	for jobResult := range jm.resultCh {
// 		handleJobResult(jobResult.JobName, jobResult.Client, jobResult)
// 		jm.Publish(jobResult.JobName, jobResult.Result)
// 	}
// }

// func main() {
// 	jm := setupJobManager()

// 	// Define a sequence of jobs to run
// 	jobSequence := []string{"jobA", "jobB", "job2", "job3"}

// 	// Start the sequence of jobs
// 	runJobSequence(jm, "client1", 10, jobSequence...)

// 	// Wait for all jobs to complete
// 	jm.wg.Wait()

// 	// Close the results channel after ensuring all jobs are done
// 	close(jm.results)

// 	// Handle graceful termination
// 	gracefulTermination()
// }

// func main() {
// 	jm := setupJobManager()

// 	// Setup subscriptions
// 	setupSubscriptions(jm)

// 	// Start the initial job with a specific argument
// 	initialInput := 10
// 	jm.StartJob("jobI", "client1", initialInput)

// 	// Start listening for job results
// 	go jm.ProcessResults()

// 	// Handle graceful termination
// 	gracefulTermination()
// }

// func (jm *JobManager) StartJob(jobName string, client string, initialArg interface{}) {
// 	jm.wg.Add(1)
// 	go func() {
// 		defer jm.wg.Done()
// 		defer atomic.AddInt32(&jm.depth, -1) // Ensure depth is decremented when job is done

// 		// Check depth and apply delay if needed
// 		depth := atomic.AddInt32(&jm.depth, 1)
// 		if depth > maxDepth {
// 			fmt.Printf("Error: Job depth exceeded maximum limit (%d).\n", maxDepth)
// 			return
// 		}
// 		if depth > safeDepth {
// 			delayFactor := depth - safeDepth
// 			delay := math.Pow(2, float64(delayFactor-1)) * float64(delayUnit)
// 			fmt.Printf("Info: Job depth is high (%d). Execution is paused for %v milliseconds to manage load.\n", depth, delay)
// 			time.Sleep(time.Duration(delay) * time.Millisecond)
// 		}

// 		// Listening for input for the job
// 		jobChannel, ok := jm.jobCh[jobName]
// 		if !ok {
// 			fmt.Printf("Error: Job channel for %s is not created.\n", jobName)
// 			return
// 		}
// 		for input, ok := <-jobChannel; ok; input, ok = <-jobChannel {
// 			result := jm.jobs[jobName].Invoke(input)
// 			result.JobName = jobName
// 			result.Client = client
// 			jm.results <- result
// 		}
// 	}()

// 	// Sending the initial argument to the job
// 	if initialArg != nil {
// 		if jobChannel, ok := jm.jobCh[jobName]; ok {
// 			jobChannel <- initialArg
// 		} else {
// 			fmt.Printf("Error: Job channel for %s is not created.\n", jobName)
// 		}
// 	}
// }

// func (jm *JobManager) StartJob(jobName string, client string, initialArg interface{}) {
// 	jm.wg.Add(1)
// 	go func() {
// 		defer jm.wg.Done()

// 		// Safely increment depth
// 		depth := atomic.AddInt32(&jm.depth, 1)
// 		defer atomic.AddInt32(&jm.depth, -1) // Ensure depth is decremented when job is done

// 		// Check depth
// 		if depth > maxDepth {
// 			fmt.Printf("Error: Job depth exceeded maximum limit (%d).\n", maxDepth)
// 			return
// 		}

// 		// Delay logic if depth exceeds safeDepth
// 		if depth > safeDepth {
// 			delayFactor := depth - safeDepth
// 			delay := math.Pow(2, float64(delayFactor-1)) * float64(delayUnit) // Exponential backoff
// 			fmt.Printf("Info: Job depth is high (%d). Execution is paused for %v milliseconds to manage load.\n", depth, delay)
// 			time.Sleep(time.Duration(delay) * time.Millisecond) // Introduce a delay
// 		}

// 		// Listening for input for the job
// 		for input, ok := <-jm.jobCh[jobName]; ok; input, ok = <-jm.jobCh[jobName] {
// 			result := jm.jobs[jobName].Invoke(input)
// 			result.JobName = jobName
// 			result.Client = client
// 			jm.results <- result
// 		}
// 	}()
// 	// Sending the initial argument to the job
// 	if initialArg != nil {
// 		jm.jobCh[jobName] <- initialArg
// 	}
// }

// // setupJobManager configures and returns a new JobManager with registered jobs.
//
//	func setupJobManager() *JobManager {
//		jm := NewJobManager()
//		jm.results = make(chan JobResult, 100) // Adjust buffer size as needed
//		jm.RegisterJob("jobA", jobA)
//		jm.RegisterJob("jobB", jobB)
//		jm.RegisterJob("job2", job2)
//		jm.RegisterJob("job3", job3)
//		jm.RegisterJob("jobI", jobI)
//		jm.RegisterJob("jobJ", jobJ)
//		return jm
//	}

// StartJob starts a job and listens for its input on a dedicated channel
// func (jm *JobManager) StartJob(jobName string, client string, initialArg interface{}) {
// 	jm.wg.Add(1)
// 	go func() {
// 		defer jm.wg.Done()

// 		// Safely increment depth
// 		depth := atomic.AddInt32(&jm.depth, 1)

// 		// Check depth
// 		if depth > maxDepth {
// 			fmt.Printf("Error: Job depth exceeded maximum limit (%d).\n", maxDepth)
// 			return
// 		}

// 		// Delay logic if depth exceeds safeDepth
// 		if depth > safeDepth {
// 			delayFactor := depth - safeDepth
// 			delay := math.Pow(2, float64(delayFactor-1)) * float64(delayUnit) // Exponential backoff
// 			fmt.Printf("Info: Job depth is high (%d). Execution is paused for %v milliseconds to manage load.\n", depth, delay)
// 			time.Sleep(time.Duration(delay) * time.Millisecond) // Introduce a delay
// 		}

// 		// Listening for input for the job
// 		for input, ok := <-jm.jobCh[jobName]; ok; input, ok = <-jm.jobCh[jobName] {
// 			result := jm.jobs[jobName].Invoke(input)
// 			result.JobName = jobName
// 			result.Client = client
// 			jm.results <- result
// 		}
// 		atomic.AddInt32(&jm.depth, -1)
// 	}()
// 	// Sending the initial argument to the job
// 	if initialArg != nil {
// 		jm.jobCh[jobName] <- initialArg
// 	}
// }

// func (jm *JobManager) StartJob(jobName string, client string, args ...interface{}) {
// 	jm.wg.Add(1) // Increment WaitGroup counter
// 	go func() {  // Launch the job in a new goroutine
// 		defer jm.wg.Done() // Decrement WaitGroup counter when goroutine completes

// 		// Safely increment depth
// 		depth := atomic.AddInt32(&jm.depth, 1)

// 		// Delay logic if depth exceeds safeDepth
// 		if depth > safeDepth {
// 			delayFactor := depth - safeDepth
// 			delay := math.Pow(2, float64(delayFactor-1)) * float64(delayUnit) // Exponential backoff
// 			fmt.Printf("Info: Job depth is high (%d). Execution is paused for %v milliseconds to manage load.\n", depth, delay)
// 			time.Sleep(time.Duration(delay) * time.Millisecond) // Introduce a delay
// 		}

// 		// Invoke the job function
// 		result := jm.jobs[jobName].Invoke(args...)
// 		result.JobName = jobName
// 		result.Client = client

// 		// Send the result to the results channel
// 		jm.results <- result

// 		// Safely decrement depth
// 		atomic.AddInt32(&jm.depth, -1)
// 	}()
// }
