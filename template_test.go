package cypress

import (
	"fmt"
	"os"
	"testing"
	"time"
)

type TestModel struct {
	Title   string
	Message string
}

func TestTemplateManager(t *testing.T) {
	SetupLogger(LogLevelDebug, os.Stdout)
	tmplMgr := NewTemplateManager("./test/tmpl", time.Second)
	tmpl, err := tmplMgr.GetOrCreateTemplate("index.tmpl", "header.tmpl")
	if err != nil {
		t.Error(err)
		return
	}

	tmpl1, err := tmplMgr.GetOrCreateTemplate("index1.tmpl", "header.tmpl")
	if err != nil {
		t.Error(err)
		return
	}

	model := &TestModel{"This is title", "This is message"}
	tmpl.Execute(os.Stdout, model)
	fmt.Println()
	tmpl1.Execute(os.Stdout, model)
	fmt.Println()

	tmpl2, err := tmplMgr.GetOrCreateTemplate("index.tmpl", "header.tmpl")
	if err != nil {
		t.Error(err)
		return
	}

	tmpl2.Execute(os.Stdout, model)

	/* you can turn on this and do live modify for header.tmpl to see if the output is updated or not
	time.Sleep(time.Second * 10)
	tmpl2, err = tmplMgr.GetOrCreateTemplate("index.tmpl", "header.tmpl")
	if err != nil {
		t.Error(err)
		return
	}

	tmpl2.Execute(os.Stdout, model)
	*/
}
