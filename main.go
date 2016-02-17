package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

const INDENT = "  "

// Port returns a string for a port role.
func Port(num int) string { return fmt.Sprintf("port%d", num) }

// Datapath returns a string for a datapath role.
func Datapath(num int) string { return fmt.Sprintf("datapath%d", num) }

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Not enough arguments (want=2, got=%d)\n", len(os.Args))
	}

	numDataPath, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatal(err.Error())
	}
	fmt.Printf("// Input: %d datapaths\n", numDataPath)
	genScribble(numDataPath)
}

func genScribble(numPorts int) {
	ports := make([]int, numPorts)
	datapaths := make([]int, numPorts)
	buf := new(bytes.Buffer)

	buf.WriteString(fmt.Sprintf("global protocol Sched%d(", numPorts))
	for i := 0; i < numPorts; i++ {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(fmt.Sprintf("role %s, roles %s", Port(i), Datapath(i)))
		ports[i] = i
		datapaths[i] = i
	}

	buf.WriteString(") {\n")
	buf.WriteString(genChoices(ports, datapaths, numPorts))
	buf.WriteString("}\n")
	fmt.Printf(buf.String())
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
