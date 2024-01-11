package main

import (
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"syscall"
	"time"
)

// JobFunc represents a generic job function.
type JobFunc interface{}

// JobResult represents the result of a job along with its metadata.
type JobResult struct {
	Result  interface{} // The actual result of the job.
	JobName string      // Name of the job.
	Client  string      // Identifier of the client that started the job.
}

// JobManager manages job functions and subscriptions.
type JobManager struct {
	jobs        map[string]JobFunc
	subscribers map[string][]string // Maps a job to its subscribers
	resultCh    chan JobResult      // Channel for job results.
}

// NewJobManager creates a new JobManager with a result channel.
func NewJobManager() *JobManager {
	return &JobManager{
		jobs:        make(map[string]JobFunc),
		subscribers: make(map[string][]string),
		resultCh:    make(chan JobResult),
	}
}

// RegisterJob registers a job with the given name and function.
func (jm *JobManager) RegisterJob(name string, job JobFunc) {
	jm.jobs[name] = job
}

// SubscribeJob sets a subscription from one job to another.
func (jm *JobManager) SubscribeJob(publisher, subscriber string) {
	jm.subscribers[publisher] = append(jm.subscribers[publisher], subscriber)
}

func (jm *JobManager) StartJob(jobName string, client string, args ...interface{}) {
	jobFunc, exists := jm.jobs[jobName]
	if !exists {
		fmt.Printf("Job %s does not exist.\n", jobName)
		return
	}

	go func() {
		reflectJobFunc := reflect.ValueOf(jobFunc)
		funcType := reflectJobFunc.Type()

		var callArgs []reflect.Value

		for i := 0; i < funcType.NumIn(); i++ {
			if i < len(args) {
				argValue := reflect.ValueOf(args[i])
				if argValue.IsValid() && argValue.Type().ConvertibleTo(funcType.In(i)) {
					callArgs = append(callArgs, argValue.Convert(funcType.In(i)))
				} else {
					callArgs = append(callArgs, reflect.Zero(funcType.In(i)))
				}
			} else {
				callArgs = append(callArgs, reflect.Zero(funcType.In(i)))
			}
		}

		results := reflectJobFunc.Call(callArgs)
		if len(results) > 0 {
			for _, result := range results {
				jm.resultCh <- JobResult{
					Result:  result.Interface(),
					JobName: jobName,
					Client:  client,
				}
			}
		} else {
			jm.resultCh <- JobResult{
				Result:  nil,
				JobName: jobName,
				Client:  client,
			}
		}
	}()
}

func (jm *JobManager) NotifySubscribersWithDelay(jobName string, client string, result interface{}, delaySec int) {
	if delaySec > 0 {
		time.Sleep(time.Duration(delaySec) * time.Second)
	}
	jm.notifySubscribers(jobName, client, result)
}

func (jm *JobManager) notifySubscribers(jobName string, client string, result interface{}) {
	for _, subJob := range jm.subscribers[jobName] {
		jm.StartJob(subJob, client, result)
	}
}

func (jm *JobManager) ListenForResults() {
	go func() {
		for jobResult := range jm.resultCh {
			fmt.Printf("Client '%s' received result from job '%s': %v\n", jobResult.Client, jobResult.JobName, jobResult.Result)

			// Introduce a delay after each job result is processed
			var delaySec int
			switch jobResult.JobName {
			case "jobAAddTwo", "jobBSubtractOne":
				delaySec = 3 // 3 seconds delay after each job
			default:
				delaySec = 0
			}

			jm.NotifySubscribersWithDelay(jobResult.JobName, jobResult.Client, jobResult.Result, delaySec)
		}
	}()
}

func (jm *JobManager) Delay(seconds int) {
	time.Sleep(time.Duration(seconds) * time.Second)
}

// Example job functions
func exampleJob(a string, b int) string {
	fmt.Printf("Example Job executing with string: %s, int: %d\n", a, b)
	return fmt.Sprintf("Processed: %s, %d", a, b)
}

func noInputReturnsOutput() []interface{} {
	fmt.Println("Job with no input but returns output")
	return []interface{}{"Output1", 42}
}

func inputNoOutput(a string, b int) {
	jm := NewJobManager()
	jm.Delay(5) // 5-second delay inside the job
	fmt.Printf("Job with input '%s', '%d' and no output\n", a, b)
}

func noInputNoOutput() {
	fmt.Println("Job with no input and no output")
}

// jobAAddTwo adds 2 to the input and sends it to jobBSubtractOne.
func jobAAddTwo(input int) int {
	result := input + 2
	fmt.Printf("jobAAddTwo: %d\n", result)
	return result
}

// jobBSubtractOne subtracts 1 from the input and sends it back to jobAAddTwo.
func jobBSubtractOne(input int) int {
	result := input - 1
	fmt.Printf("jobBSubtractOne: %d\n", result)
	return result
}

func main() {
	jm := NewJobManager()

	//test mode
	// Register jobs
	jm.RegisterJob("jobAAddTwo", jobAAddTwo)
	jm.RegisterJob("jobBSubtractOne", jobBSubtractOne)

	// Define subscriptions
	jm.SubscribeJob("jobAAddTwo", "jobBSubtractOne")
	jm.SubscribeJob("jobBSubtractOne", "jobAAddTwo")

	// Define a client identifier
	client := "client"
	clientx := "clientx"

	// Start the loop with an initial value
	initialInput := 10
	jm.StartJob("jobAAddTwo", client, initialInput)

	// Delay before starting the loop for 'clientx'
	delayInSeconds := 5 // Delay for 5 seconds
	time.Sleep(time.Duration(delayInSeconds) * time.Second)

	initialInputx := 10000
	jm.StartJob("jobBSubtractOne", clientx, initialInputx)

	// Register jobs
	// jm.RegisterJob("exampleJob", exampleJob)
	// jm.RegisterJob("noInputReturnsOutput", noInputReturnsOutput)
	// jm.RegisterJob("inputNoOutput", inputNoOutput)
	// jm.RegisterJob("noInputNoOutput", noInputNoOutput)

	//jm.ListenForResults()

	// Define subscriptions
	// jm.SubscribeJob("exampleJob", "noInputReturnsOutput")
	// jm.SubscribeJob("noInputReturnsOutput", "inputNoOutput")
	// jm.SubscribeJob("inputNoOutput", "noInputNoOutput")
	// jm.SubscribeJob("noInputNoOutput", "inputNoOutput")

	// // Define client identifiers
	// client1 := "client1"
	// client2 := "client2"
	// client3 := "client3"
	// client4 := "client4"

	// // Start jobs with client identifiers
	// jm.StartJob("exampleJob", client1, "Hello", 123)
	// jm.StartJob("noInputReturnsOutput", client2)
	// jm.StartJob("inputNoOutput", client3, "Test", 100)
	// jm.StartJob("noInputNoOutput", client4)

	jm.ListenForResults()

	// Handle graceful termination
	gracefulTermination()
}

func gracefulTermination() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	fmt.Println("Program exiting gracefully")
}
