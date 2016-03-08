package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

const (
	DATAPATH_PREFIX = "idx" // Input prefix.
	PORT_PREFIX     = "p"
)

type Connection struct {
	port     int
	datapath int
}

func (c Connection) String() string {
	return fmt.Sprintf("{port: %d, datapath: %d}", c.port, c.datapath)
}

// Port returns a string for a port role.
func Port(num int) string { return fmt.Sprintf("%s%d", PORT_PREFIX, num) }

// Datapath returns a string for a datapath role.
func Datapath(num int) string { return fmt.Sprintf("%s%d", DATAPATH_PREFIX, num) }

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Not enough arguments (want=2, got=%d)\n", len(os.Args))
	}

	numDataPath, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatal(err.Error())
	}

	fmt.Printf("// Input: %d datapaths\n", numDataPath)

	scribble, err := os.OpenFile("sched.scr", os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}
	defer func() {
		scribble.Close()
	}()

	cFile, err := os.OpenFile("sched.c", os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}
	defer func() {
		cFile.Close()
	}()

	scribble.WriteString(genScribble(numDataPath))
	cFile.WriteString(genSched(numDataPath))
}

func genScribble(N int) string {
	buf := new(bytes.Buffer)

	buf.WriteString(fmt.Sprintf("module Sched;\nglobal protocol Sched%d(", N))
	for i := 0; i < N; i++ {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(fmt.Sprintf("role %s, role %s", Port(i), Datapath(i)))
	}
	buf.WriteString(") {\n") // Protocol block.

	base := make([][]Connection, N)
	msgPris := make([][]int, N) // Priorities of each pair.

	for port := 0; port < N; port++ {
		msgPris[port] = make([]int, N)
		base[port] = make([]Connection, N)
		for priority := 0; priority < N; priority++ {
			datapath := (port + priority) % N
			base[port][priority] = Connection{port: port, datapath: datapath}

			// Assign predefined priority to each port-datapath pair.
			msgPris[port][datapath] = priority
		}
	}

	path := make([]string, N)
	for port := 0; port < N; port++ {
		for priority := 0; priority < N; priority++ {
			datapath := (port + priority) % N
			path[port] += fmt.Sprintf("choice at %s {\n", Datapath(datapath))
			path[port] += fmt.Sprintf("  use%d_%d() from %s to %s;\n", datapath, port, Datapath(base[port][msgPris[port][datapath]].datapath), Port(base[port][msgPris[port][datapath]].port))
			for i := priority; i < N-1; i++ {
				path[port] += fmt.Sprintf("  T_%d_%d() from %s to %s; // Propagate\n", datapath, port, Datapath(datapath), Datapath((port+(i+1))%N))
			}
			path[port] += fmt.Sprintf("} or {\n")
			path[port] += fmt.Sprintf("  off%d_%d() from %s to %s;\n", datapath, port, Datapath(base[port][msgPris[port][datapath]].datapath), Port(base[port][msgPris[port][datapath]].port))
			for i := priority; i < N-1; i++ {
				path[port] += fmt.Sprintf("  F_%d_%d() from %s to %s; // Propagate\n", datapath, port, Datapath(datapath), Datapath((port+(i+1))%N))
			}
		}
		path[port] += fmt.Sprintf("%s\n", strings.Repeat("}", N))
	}

	for port := 0; port < N; port++ {
		buf.WriteString(fmt.Sprintf("// Path %d\n%s\n", port, path[port]))
	}

	buf.WriteString("}\n") // Protocol block.
	return buf.String()
}

// notMsg is an expression to calculate (datapath % N == port) optimised for H/W
func notMsg(datapath, port, N int) string {
	if N > 0 && (port&(port-1) == 0) { // Power of 2
		return fmt.Sprintf("((%s&%d)^%d)", Datapath(datapath), N-1, port)
	} else {
		return fmt.Sprintf("((%s%%%d)^%d)", Datapath(datapath), N, port)
	}
}

// genSched generates a scheduler file.
func genSched(N int) string {
	buf := new(bytes.Buffer)

	buf.WriteString("#include <stdlib.h>\n") // size_t

	baseCond := make([][]string, N) // Base connection pairs (negated).
	msgPris := make([][]int, N)     // Priorities of each pair.

	for port := 0; port < N; port++ {
		baseCond[port] = make([]string, N)
		msgPris[port] = make([]int, N)
		for priority := 0; priority < N; priority++ {
			datapath := (port + priority) % N
			baseCond[port][priority] = notMsg(datapath, port, N)

			// Assign predefined priority to each port-datapath pair.
			msgPris[port][datapath] = priority
		}
	}

	cond := make([][]string, N)
	for port := 0; port < N; port++ {
		cond[port] = make([]string, N)
		for datapath := 0; datapath < N; datapath++ {
			cond[port][datapath] = fmt.Sprintf("!(%s)", baseCond[port][msgPris[port][datapath]])
			for priority := msgPris[port][datapath] - 1; priority >= 0; priority-- {
				cond[port][datapath] += fmt.Sprintf(" && %s", baseCond[port][priority])
			}
		}
	}

	buf.WriteString(fmt.Sprintf("void sched%d(", N))
	for i := 0; i < N; i++ {
		buf.WriteString(fmt.Sprintf("size_t %s, ", Datapath(i)))
	}
	buf.WriteString(fmt.Sprintf("int *enabled) {\n"))

	for port := 0; port < N; port++ {
		for datapath := 0; datapath < N; datapath++ {
			buf.WriteString(fmt.Sprintf("  enabled[%d * %d + %d] = %s;\n", port, N, datapath, cond[port][datapath]))
		}
		buf.WriteString("\n")
	}

	buf.WriteString("}\n")
	return buf.String()
}
