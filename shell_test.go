package scipipe

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"os"
	t "testing"
)

func TestShellHasInOutPorts(t *t.T) {
	tt := Sh("echo {i:in1} {o:out1}")
	tt.OutPathFuncs["out1"] = func() string {
		return fmt.Sprint(tt.InPaths["in1"], ".bar")
	}

	tt.InPorts["in1"] = make(chan *FileTarget, BUFSIZE)
	go func() { tt.InPorts["in1"] <- NewFileTarget("foo.txt") }()
	go tt.Run()
	<-tt.OutPorts["out1"]

	assert.NotNil(t, tt.InPorts["in1"], "InPorts are nil!")
	assert.NotNil(t, tt.OutPorts["out1"], "OutPorts are nil!")

	cleanFiles("foo.txt", "foo.txt.bar")
}

func TestShellCloseOutPortOnInPortClose(t *t.T) {
	fooTask := Sh("echo foo > {o:out1}")
	fooTask.OutPathFuncs["out1"] = func() string {
		return "foo.txt"
	}

	barReplacer := Sh("sed 's/foo/bar/g' {i:foo} > {o:bar}")
	barReplacer.OutPathFuncs["bar"] = func() string {
		return barReplacer.GetInPath("foo") + ".bar"
	}

	barReplacer.InPorts["foo"] = fooTask.OutPorts["out1"]

	go fooTask.Run()
	go barReplacer.Run()
	<-barReplacer.OutPorts["bar"]

	// Assert no more content coming on channels
	assert.Nil(t, <-fooTask.OutPorts["out1"])
	assert.Nil(t, <-barReplacer.OutPorts["bar"])

	_, fooErr := os.Stat("foo.txt")
	assert.Nil(t, fooErr)
	_, barErr := os.Stat("foo.txt.bar")
	assert.Nil(t, barErr)

	cleanFiles("foo.txt", "foo.txt.bar")
}

func TestReplacePlaceholdersInCmd(t *t.T) {
	rawCmd := "echo {i:in1} > {o:out1}"
	tt := Sh(rawCmd)
	tt.OutPathFuncs["out1"] = func() string {
		return fmt.Sprint(tt.InPaths["in1"], ".bar")
	}

	tt.InPorts["in1"] = make(chan *FileTarget, BUFSIZE)
	ift := NewFileTarget("foo.txt")
	go func() {
		defer close(tt.InPorts["in1"])
		tt.InPorts["in1"] <- ift
	}()

	// Assert inport is still open after first read
	inPortsClosed := tt.receiveInputs()
	assert.Equal(t, false, inPortsClosed)

	// Assert inport is closed after second read
	inPortsClosed = tt.receiveInputs()
	assert.Equal(t, true, inPortsClosed)

	// Assert InPath is correct
	assert.Equal(t, "foo.txt", tt.InPaths["in1"], "foo.txt")

	// Assert placeholders are correctly replaced in command
	cmd := tt.formatCommand(rawCmd)
	assert.EqualValues(t, "echo foo.txt > foo.txt.bar", cmd)
}

func TestParameterCommand(t *t.T) {
	cmb := NewCombinatoricsTask()

	// An abc file printer
	abc := Sh("echo {p:a} {p:b} {p:c} > {o:out}")
	abc.OutPathFuncs["out"] = func() string {
		return fmt.Sprintf(
			"%s_%s_%s.txt",
			abc.Params["a"],
			abc.Params["b"],
			abc.Params["c"],
		)
	}

	// A printer task
	prt := Sh("cat {i:in} >> /tmp/log.txt; rm {i:in}")

	// Connection info
	abc.ParamPorts["a"] = cmb.A
	abc.ParamPorts["b"] = cmb.B
	abc.ParamPorts["c"] = cmb.C
	prt.InPorts["in"] = abc.OutPorts["out"]

	pl := NewPipeline()
	pl.AddTasks(cmb, abc, prt)
	pl.Run()

	// Run tests
	_, err := os.Stat("/tmp/log.txt")
	assert.Nil(t, err)

	cleanFiles("/tmp/log.txt")
}

func TestTaskWithoutInputsOutputs(t *t.T) {
	f := "/tmp/hej.txt"
	tsk := Sh("echo hej > " + f)
	tsk.Run()
	_, err := os.Stat(f)
	assert.Nil(t, err)
	cleanFiles(f)
}

// Helper functions

func cleanFiles(fileNames ...string) {
	for _, fileName := range fileNames {
		if _, err := os.Stat(fileName); err == nil {
			fmt.Println("Removing file", fileName)
			os.Remove(fileName)
			fmt.Println("Successfully removed file", fileName)
		}
	}
}

// Helper tasks

type CombinatoricsTask struct {
	BaseTask
	A chan string
	B chan string
	C chan string
}

func NewCombinatoricsTask() *CombinatoricsTask {
	return &CombinatoricsTask{
		A: make(chan string, BUFSIZE),
		B: make(chan string, BUFSIZE),
		C: make(chan string, BUFSIZE),
	}
}

func (proc *CombinatoricsTask) Run() {
	defer close(proc.A)
	defer close(proc.B)
	defer close(proc.C)

	for _, a := range SS("a1", "a2", "a3") {
		for _, b := range SS("b1", "b2", "b3") {
			for _, c := range SS("c1", "c2", "c3") {
				proc.A <- a
				proc.B <- b
				proc.C <- c
			}
		}
	}
}
