package main

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"time"

	"github.com/karalabe/bufioprop"
	"github.com/karalabe/bufioprop/shootout/mattharden"
	"github.com/karalabe/bufioprop/shootout/rogerpeppe"
)

type copyFunc func(dst io.Writer, src io.Reader, buffer int) (int64, error)

type contender struct {
	Name string
	Copy copyFunc
}

var contenders = []contender{
	// First contender is the build in io.Copy (wrapped in out specific signature)
	{"io.Copy", func(dst io.Writer, src io.Reader, buffer int) (int64, error) {
		return io.Copy(dst, src)
	}},
	// Second contender is the proposed bufio.Copy (currently at bufioprop.Copy)
	{"bufio.Copy", bufioprop.Copy},

	// Other contenders written by mailing list contributions
	{"rogerpeppe.Copy", rogerpeppe.Copy},
	{"mattharden.Copy", mattharden.Copy},
}

func main() {
	// Generate a random data source long enough to discover the issues
	src := rand.NewSource(0)
	data := make([]byte, 32*1024*1024)
	for i := 0; i < len(data); i++ {
		data[i] = byte(src.Int63() & 0xff)
	}
	// Simulate copying between various types of readers and writers
	fmt.Println("Stable input, stable output:")
	for _, copier := range contenders {
		in, out := stableInput(data), stableOutput()
		benchmark(in, out, len(data), copier)
	}
	fmt.Println()

	fmt.Println("Stable input, bursty output:")
	for _, copier := range contenders {
		in, out := stableInput(data), burstyOutput()
		benchmark(in, out, len(data), copier)
	}
	fmt.Println()

	fmt.Println("Bursty input, stable output:")
	for _, copier := range contenders {
		in, out := burstyInput(data), stableOutput()
		benchmark(in, out, len(data), copier)
	}
	fmt.Println()
}

// Benchmark runs a copy operation on the given input/output endpoints with the
// specified copy function.
func benchmark(r io.Reader, w io.Writer, size int, copier contender) {
	buffer := 1024 * 1024

	start := time.Now()
	if n, err := copier.Copy(w, r, buffer); int(n) != size || err != nil {
		fmt.Printf("%10s: operation failed: have n %d, want n %d, err %v.\n", copier.Name, n, size, err)
		return
	}
	elapsed := time.Since(start)
	throughput := float64(size) / (1024 * 1024) / float64(elapsed/time.Second)
	fmt.Printf("%15s: %14v %10f mbps.\n", copier.Name, elapsed, throughput)
}

// StableInput creates a 10MBps data source streaming stably in small chunks of
// 100KB each.
func stableInput(data []byte) io.Reader {
	return input(10*time.Millisecond, 100*1024, data)
}

// BurstyInput creates a 10MBps data source streaming in bursts of 1MB.
func burstyInput(data []byte) io.Reader {
	return input(100*time.Millisecond, 1000*1024, data)
}

// StableOutput creates a 10MBps data sink consuming stably in small chunks of
// 100KB each.
func stableOutput() io.Writer {
	return output(10*time.Millisecond, 100*1024)
}

// BurstyOutput creates a 10MBps data sink consuming in bursts of 1MB.
func burstyOutput() io.Writer {
	return output(100*time.Millisecond, 1000*1024)
}

// Input creates an unbuffered data source, filled at the specified rate.
func input(cycle time.Duration, chunk int, data []byte) io.Reader {
	source := bytes.NewBuffer(data)
	buffer := make([]byte, chunk)
	pr, pw := io.Pipe()

	// Input generator that will produce data at a specified rate
	go func() {
		defer pw.Close()

		for {
			// Make the next chunk available in the input stream
			n, err := io.ReadFull(source, buffer)
			if n > 0 {
				if _, err := pw.Write(buffer[:n]); err != nil {
					panic(err)
				}
			}
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return
			} else if err != nil {
				panic(err)
			}
			// Sleep a while to simulate throughput
			time.Sleep(cycle)
		}
	}()
	return pr
}

// Input creates an unbuffered data sink, emptied at the specified rate.
func output(cycle time.Duration, chunk int) io.Writer {
	buffer := make([]byte, chunk)
	pr, pw := io.Pipe()

	// Output reader that will consume data at a specified rate
	go func() {
		defer pr.Close()

		for {
			// Consume the next chunk from the output stream
			_, err := io.ReadFull(pr, buffer)
			if err == io.EOF {
				return
			} else if err != nil {
				panic(err)
			}
			// Sleep a while to simulate throughput
			time.Sleep(cycle)
		}
	}()
	return pw
}