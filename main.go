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
	INDENT          = "  "
	DATAPATH_PREFIX = "idx" // Input prefix.
	PORT_PREFIX     = "p"
)

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

func genScribble(numPorts int) string {
	ports := make([]int, numPorts)
	datapaths := make([]int, numPorts)
	buf := new(bytes.Buffer)

	buf.WriteString(fmt.Sprintf("global protocol Sched%d(", numPorts))
	for i := 0; i < numPorts; i++ {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(fmt.Sprintf("role %s, role %s", Port(i), Datapath(i)))
		ports[i] = i
		datapaths[i] = i
	}

	buf.WriteString(") {\n")
	buf.WriteString(genChoices(ports, datapaths, numPorts))
	buf.WriteString("}\n")
	return buf.String()
}

// genChoiceBody generates choice block bodies given the selected port and
// datapath, with the total number of datapaths available.
func genChoiceBody(port, datapath, totalDatapath int) string {
	buf := new(bytes.Buffer)
	for i := 0; i < totalDatapath; i++ {
		buf.WriteString(strings.Repeat(INDENT, port+2))
		var stmt string
		if datapath == i {
			stmt = fmt.Sprintf("Use%d_%d() from %s to %s;\n", datapath, port, Port(port), Datapath(i))
		} else {
			stmt = fmt.Sprintf("Off%d_%d() from %s to %s;\n", datapath, port, Port(port), Datapath(i))
		}
		if _, err := buf.WriteString(stmt); err != nil {
			log.Fatal(err)
		}
	}
	return buf.String()
}

// genChoices generates choice blocks (recursively) from the list of free and
// unused ports/datapaths.
func genChoices(freePorts, freeDPs []int, totalDatapath int) string {
	buf := new(bytes.Buffer)
	numPorts := len(freePorts)
	numDPs := len(freeDPs)

	if numPorts == 0 || numDPs == 0 {
		return ""
	}

	freePortsAfter := make([]int, numPorts-1)
	copy(freePortsAfter, freePorts[1:])
	buf.WriteString(strings.Repeat(INDENT, totalDatapath-numPorts+1))
	buf.WriteString(fmt.Sprintf("choice at %s {\n", Port(freePorts[0])))

	for dp := 0; dp < numDPs; dp++ {
		freeDPsAfter := make([]int, numDPs)
		copy(freeDPsAfter, freeDPs)
		freeDPsAfter = append(freeDPsAfter[:dp], freeDPsAfter[dp+1:]...)

		buf.WriteString(genChoiceBody(freePorts[0], freeDPs[dp], totalDatapath))
		if numPorts > 1 {
			buf.WriteString(strings.Repeat(INDENT, totalDatapath-numPorts+2))
			// Scribble: a message to 'hand over' decision to the next choice-sender
			buf.WriteString(fmt.Sprintf("Next%d_%d() from %s to %s;\n", freeDPs[dp], freePorts[0], Port(freePorts[0]), Port(freePortsAfter[0])))
			buf.WriteString(genChoices(freePortsAfter, freeDPsAfter, totalDatapath))
		}

		// Separator
		if dp < numDPs-1 {
			buf.WriteString(strings.Repeat(INDENT, totalDatapath-numPorts+1) + "} or {\n")
		}
	}

	buf.WriteString(strings.Repeat(INDENT, totalDatapath-numPorts+1) + "}\n")

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
