package cypress

import (
	"encoding/json"
	"html/template"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"
)

type TestModel struct {
	Title   string
	Message string
}

func readBuffer(buf [][]byte) string {
	result := ""
	for _, b := range buf {
		result += string(b)
	}

	return result
}

func TestTemplateManager(t *testing.T) {
	// test setup
	// create test folder
	testDir, err := ioutil.TempDir("", "cytpltest")
	if err != nil {
		t.Error("failed to create test dir", err)
		return
	}

	defer os.RemoveAll(testDir)

	// write template files
	err = ioutil.WriteFile(path.Join(testDir, "header.tmpl"), []byte("{{.}}"), os.ModePerm)
	if err != nil {
		t.Error("failed to setup header.tmpl")
		return
	}

	err = ioutil.WriteFile(path.Join(testDir, "index.tmpl"), []byte("{{template \"header.tmpl\" .Title}}{{.Message}}{{add 1 1}}"), os.ModePerm)
	if err != nil {
		t.Error("failed to setup index.tmpl")
		return
	}

	err = ioutil.WriteFile(path.Join(testDir, "index1.tmpl"), []byte("{{template \"header.tmpl\" .Title}}{{.Message}}"), os.ModePerm)
	if err != nil {
		t.Error("failed to setup index1.tmpl")
		return
	}

	writer := NewBufferWriter()
	SetupLogger(LogLevelDebug, writer)

	funcMap := template.FuncMap{
		"add": func(a, b int32) int32 { return a + b },
	}

	tmplMgr := NewTemplateManager(testDir, time.Second).Funcs(funcMap)
	tmpl, err := tmplMgr.GetOrCreateTemplate("index.tmpl", "header.tmpl")
	if err != nil {
		t.Error("failed to get index.tmpl template", err)
		return
	}

	tmpl1, err := tmplMgr.GetOrCreateTemplate("index1.tmpl", "header.tmpl")
	if err != nil {
		t.Error("failed to get index1.tmpl template", err)
		return
	}

	resultWriter := NewBufferWriter()
	model := &TestModel{"title", "message"}
	tmpl.Execute(resultWriter, model)
	result := readBuffer(resultWriter.Buffer)
	if result != "titlemessage2" {
		t.Error("expected titlemessage2 but got", result)
		return
	}

	resultWriter = NewBufferWriter()
	tmpl1.Execute(resultWriter, model)
	result = readBuffer(resultWriter.Buffer)

	if result != "titlemessage" {
		t.Error("expected titlemessage but got", result)
		return
	}

	_, err = tmplMgr.GetOrCreateTemplate("index.tmpl", "header.tmpl")
	if err != nil {
		t.Error("failed to get index.tmpl template", err)
		return
	}

	if len(writer.Buffer) != 1 {
		t.Error("expected one log entry but got", len(writer.Buffer))
		return
	}

	type log struct {
		Level   string `json:"level"`
		Message string `json:"msg"`
		Name    string `json:"name"`
	}

	l := log{}
	err = json.Unmarshal(writer.Buffer[0], &l)
	if err != nil {
		t.Error("bad log entry", err)
		return
	}

	if l.Level != "debug" || l.Message != "templateCacheHit" || l.Name != "index.tmpl" {
		t.Error("unexpected log entry", string(writer.Buffer[0]))
		return
	}

	// test reload
	// don't change too quick
	time.Sleep(time.Millisecond * 50)
	err = ioutil.WriteFile(path.Join(testDir, "header.tmpl"), []byte("{{.}}updated"), os.ModePerm)
	if err != nil {
		t.Error("failed to update header.tmpl")
		return
	}

	time.Sleep(time.Second * 2)
	tmpl2, err := tmplMgr.GetOrCreateTemplate("index.tmpl", "header.tmpl")
	if err != nil {
		t.Error(err)
		return
	}

	resultWriter = NewBufferWriter()
	tmpl2.Execute(resultWriter, model)
	result = readBuffer(resultWriter.Buffer)
	if result != "titleupdatedmessage2" {
		t.Error("expected titleupdatedmessage2 but got", result)
		return
	}
}
