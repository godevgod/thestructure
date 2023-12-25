//The Stucture make for server side - goroutine not deadlock

package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Job func(args ...interface{}) (result interface{}, err error)

type JobStatus struct {
	Running   bool
	StartTime time.Time
	EndTime   time.Time
}

var (
	jobs                  = make(map[string]Job)
	jobStatuses           = make(map[string]*JobStatus)
	jobCompletionChannels = make(map[string]chan string)
	mu                    sync.Mutex // To handle concurrency
)

func initializeJobStatuses(jobNames []string) {
	for _, name := range jobNames {
		jobStatuses[name] = &JobStatus{Running: false}
		jobCompletionChannels[name] = make(chan string)
	}
}

func executeJob(name string, jobFunc Job, args ...interface{}) {
	fmt.Printf("Job %s is starting.\n", name)
	jobStatuses[name].Running = true
	jobStatuses[name].StartTime = time.Now()

	result, err := jobFunc(args...)
	if err != nil {
		fmt.Printf("Error executing job %s: %v\n", name, err)
	}

	jobStatuses[name].Running = false
	jobStatuses[name].EndTime = time.Now()
	jobCompletionChannels[name] <- name
	fmt.Printf("Job %s completed with result: %v\n", name, result)
}

func startJobByName(jobName string) {
	jobFunc, exists := jobs[jobName]
	if !exists {
		fmt.Printf("Cannot start job %s because it doesn't exist.\n", jobName)
		return
	}
	go executeJob(jobName, jobFunc)
}

func listenForJobs(triggeringJob string, dependentJobs ...string) {
	go func() {
		for jobName := range jobCompletionChannels[triggeringJob] {
			fmt.Printf("Job %s has completed. Checking for dependent jobs to trigger.\n", jobName)
			for _, dependentJob := range dependentJobs {
				mu.Lock()
				if !jobStatuses[dependentJob].Running && shouldTriggerNextJob(jobName, dependentJob, jobStatuses[jobName].EndTime) {
					startJobByName(dependentJob)
				}
				mu.Unlock()
			}
		}
	}()
}
func main() {
	// Define jobs and their dependencies.
	jobs["jobA"] = jobA
	jobs["jobB"] = jobB
	jobs["jobC"] = jobC
	jobs["jobD"] = jobD
	jobs["jobE"] = jobE
	jobs["jobF"] = jobF
	jobs["jobG"] = jobG
	initializeJobStatuses([]string{"jobA", "jobB", "jobC", "jobD", "jobE", "jobF", "jobG"})

	// Listen for jobA's completion to trigger jobB and jobC.
	// Set up listeners for job triggers
	listenForJobs("jobA", "jobB", "jobC") // Job A triggers B and C
	listenForJobs("jobC", "jobD")         // Job C triggers D
	//time.Sleep(100 * time.Second)
	listenForJobs("jobD", "jobC") // Job D triggers C

	// Start jobA.
	startJobByName("jobA")

	// Prevent the main function from exiting immediately.
	time.Sleep(10 * time.Second)

	// Check and start jobE based on external condition
	// Check and start conditional jobs
	go checkAndStartConditionalJobs()

	// Graceful termination.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	fmt.Println("Graceful termination initiated.")
	os.Exit(0)
}
func jobA(args ...interface{}) (interface{}, error) {
	fmt.Println("Starting jobA")
	countdown("jobA", 5)
	time.Sleep(2 * time.Second)
	return "Special Result from jobA", nil
}

func jobB(args ...interface{}) (interface{}, error) {
	fmt.Println("Starting jobB")
	countdown("jobB", 3)
	time.Sleep(1 * time.Second)
	return "Job B Result", nil
}

func jobC(args ...interface{}) (interface{}, error) {
	fmt.Println("Job C is triggered and running.")
	countdown("jobC", 4)
	time.Sleep(1 * time.Second)
	return "Job C Result", nil
}

func jobD(args ...interface{}) (interface{}, error) {
	fmt.Println("Starting jobD")
	countdown("jobD", 5)
	time.Sleep(2 * time.Second)
	return "Job D Result", nil
}

func jobE(args ...interface{}) (interface{}, error) {
	fmt.Println("Starting jobE")
	countdown("jobE", 5)
	time.Sleep(2 * time.Second)
	return "Job E Result", nil
}

func jobF(args ...interface{}) (interface{}, error) {
	// Job F implementation
	fmt.Println("Starting jobF")
	countdown("jobF", 3)
	time.Sleep(1 * time.Second)
	return "Job F Result", nil
}

func jobG(args ...interface{}) (interface{}, error) {
	// Job G implementation
	fmt.Println("Starting jobG")
	countdown("jobG", 2)
	time.Sleep(1 * time.Second)
	return "Job G Result", nil
}

func checkAndStartConditionalJobs() {
	// Define the list of conditional jobs and their specific triggers
	conditionalJobs := map[string]func() bool{
		"jobE": shouldStartJobE,
		"jobF": shouldStartJobF,
		"jobG": shouldStartJobG,
	}

	for {
		time.Sleep(1 * time.Second) // Check every second
		for jobName, triggerFunc := range conditionalJobs {
			if !jobStatuses[jobName].Running && triggerFunc() {
				startJobByName(jobName)
			}
		}
	}
}

func triggerNextJobs(completedJob string, result interface{}) {
	for jobName := range jobCompletionChannels {
		if jobName == completedJob {
			continue
		}

		// Condition to trigger the next job - can be customized as needed.
		if shouldTriggerNextJob(completedJob, jobName, result) {
			startJobByName(jobName)
		}
	}
}

// shouldTriggerNextJob checks if the next job should be triggered based on custom logic.
func shouldTriggerNextJob(completedJob, nextJob string, result interface{}) bool {
	// Implement complex conditions for each job trigger scenario
	if completedJob == "jobA" && (nextJob == "jobB" || nextJob == "jobC") {
		return true // Trigger jobB and jobC when jobA completes with any result
	}
	if completedJob == "jobC" && nextJob == "jobD" {
		return true // Trigger jobD when jobC completes
	}
	if completedJob == "jobD" && nextJob == "jobC" && !jobStatuses["jobC"].Running {
		return true // Restart jobC when jobD completes, only if jobC is not running
	}

	// Logic for triggering jobE based on external condition and jobB completion
	if nextJob == "jobE" {
		// Check if jobB has completed at least once
		if jobStatuses["jobB"].EndTime.After(jobStatuses["jobB"].StartTime) {
			currentUnixTime := time.Now().Unix()
			// Check if the current Unix time is divisible by 10
			if currentUnixTime%10 == 0 {
				fmt.Printf("Triggering jobE as the Unix time %d is divisible by 10.\n", currentUnixTime)
				return true
			}
		}
		return false
	}
	return false
}

// countdown function that can be used by any job
func countdown(jobName string, duration int) {
	fmt.Printf("Starting %s with countdown:\n", jobName)
	for i := duration; i > 0; i-- {
		fmt.Printf("%s countdown: %d\n", jobName, i)
		time.Sleep(1 * time.Second) // Sleep for one second between counts
	}
}

// shouldStartJobE checks if jobE should be triggered based on specific logic.
func shouldStartJobE() bool {
	// Example: Trigger jobE if jobB has completed and current Unix time is divisible by 10
	if jobStatuses["jobB"].EndTime.After(jobStatuses["jobB"].StartTime) {
		return time.Now().Unix()%10 == 0
	}
	return false
}

// shouldStartJobF checks if jobF should be triggered based on specific logic.
func shouldStartJobF() bool {
	// Example: Trigger jobF if jobC has completed and current minute is even
	if jobStatuses["jobC"].EndTime.After(jobStatuses["jobC"].StartTime) {
		return time.Now().Minute()%2 == 0
	}
	return false
}

// shouldStartJobG checks if jobG should be triggered based on specific logic.
func shouldStartJobG() bool {
	// Example: Trigger jobG if current hour is odd
	return time.Now().Hour()%2 != 0
}
