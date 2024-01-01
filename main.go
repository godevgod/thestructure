//main.go golang code

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Job func(ctx context.Context, args ...interface{}) (result interface{}, err error)

var (
	jobs           = make(map[string]Job)
	jobStatuses    = make(map[string]*JobStatus)
	jobSubscribers = make(map[string][]string)
	mu             sync.Mutex // To handle concurrency
)

type JobStatus struct {
	Running   bool
	StartTime time.Time
	EndTime   time.Time
}

func initializeJobStatuses(jobNames []string) {
	for _, name := range jobNames {
		jobStatuses[name] = &JobStatus{Running: false}
	}
}

func startJobByName(jobName string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	jobFunc, exists := jobs[jobName]
	if !exists {
		fmt.Printf("Job %s does not exist.\n", jobName)
		return
	}

	if jobStatuses[jobName].Running {
		fmt.Printf("Job %s is already running.\n", jobName)
		return
	}

	// สร้าง context ที่ไม่มีการยกเลิกหรือ timeout
	ctx := context.Background()

	//go executeJob(jobName, jobFunc, args...)
	go executeJob(ctx, jobName, jobFunc, args...)
}

func executeJob(ctx context.Context, name string, jobFunc Job, args ...interface{}) {
	fmt.Printf("Starting job %s.\n", name)
	jobStatuses[name].Running = true
	jobStatuses[name].StartTime = time.Now()

	// ทำงานของ job
	result, err := jobFunc(ctx, args...)

	jobStatuses[name].Running = false
	jobStatuses[name].EndTime = time.Now()

	if err != nil {
		fmt.Printf("Error in job %s: %v\n", name, err)
		return
	}

	fmt.Printf("Job %s completed with result: %v\n", name, result)

	// Notify subscribers
	notifySubscribers(name, result)
}

func notifySubscribers(jobName string, result interface{}) {
	for _, subscriber := range jobSubscribers[jobName] {
		// ส่งข้อมูลเกี่ยวกับงานที่เรียก (jobName) ไปยัง subscriber
		startJobByName(subscriber, jobName, result)
	}
}

func job1(ctx context.Context, args ...interface{}) (interface{}, error) {
	fmt.Println("Executing job1")
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(3 * time.Second): // Simulate work
		return "Result from job1", nil
	}
}

func job2(ctx context.Context, args ...interface{}) (interface{}, error) {
	caller := args[0] // ข้อมูลเกี่ยวกับงานที่เรียกใช้งานนี้
	fmt.Printf("Executing job2, called by %v\n", caller)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(4 * time.Second): // Simulate work
		return fmt.Sprintf("Result from job2, time: %v", time.Now().Unix()), nil
	}
}

var count int64 = 0 // Global counter for job3

func job3(ctx context.Context, args ...interface{}) (interface{}, error) {
	caller := args[0] // ข้อมูลเกี่ยวกับงานที่เรียกใช้งานนี้
	fmt.Printf("Executing job3, called by %v\n", caller)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second): // Simulate work
		count++ // Increment the counter
		return fmt.Sprintf("Result from job3, count: %d", count), nil
	}
}

func main() {
	initializeJobStatuses([]string{"job1", "job2", "job3"})

	jobs["job1"] = job1
	jobs["job2"] = job2
	jobs["job3"] = job3

	// Setting up subscribers
	jobSubscribers["job1"] = []string{"job2"}
	jobSubscribers["job2"] = []string{"job3"}
	jobSubscribers["job3"] = []string{"job2"}

	// Start the first job
	startJobByName("job1")

	// Keep the application running to observe job execution
	gracefulTermination()
	//	for {
	//time.Sleep(9000 * time.Second)
	//	}
}

// gracefulTermination handles the graceful termination of the program
func gracefulTermination() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Block until a signal is received.
	<-sigChan

	// Perform cleanup here if needed

	fmt.Println("Program exiting gracefully")
}

/*
// jobNew คือฟังก์ชันงานใหม่ที่คุณต้องการเพิ่ม
func jobNew(ctx context.Context, args ...interface{}) (interface{}, error) {
	// ตรงนี้ใส่โค้ดของงานใหม่ที่คุณต้องการ
	// ตัวอย่าง: ใช้เวลาในการทำงานและส่งคืนค่าผลลัพธ์
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(2 * time.Second): // Simulate work
		return "Result from jobNew", nil
	}
}


   // เพิ่มสถานะของงานใหม่
   initializeJobStatuses([]string{"job1", "job2", "job3", "jobNew"}) // เพิ่ม "jobNew"

   // ลงทะเบียนงานใหม่
   jobs["jobNew"] = jobNew

   // ตั้งค่า Subscriber สำหรับ jobNew ถ้ามี
   jobSubscribers["jobNew"] = []string{"job1", "job2"} // ตัวอย่าง: ให้ job1 และ job2 ทำงานหลังจาก jobNew
*/
