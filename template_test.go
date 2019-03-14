package cypress

import (
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
	err = ioutil.WriteFile(path.Join(testDir, "header.tmpl"), []byte("{{define \"header\"}}{{.}}{{end}}"), os.ModePerm)
	if err != nil {
		t.Error("failed to setup header.tmpl")
		return
	}

	err = ioutil.WriteFile(path.Join(testDir, "index.tmpl"), []byte("{{define \"index\"}}{{template \"header\" .Title}}{{.Message}}{{add 1 1}}{{end}}"), os.ModePerm)
	if err != nil {
		t.Error("failed to setup index.tmpl")
		return
	}

	err = ioutil.WriteFile(path.Join(testDir, "index1.tmpl"), []byte("{{define \"index1\"}}{{template \"header\" .Title}}{{.Message}}{{end}}"), os.ModePerm)
	if err != nil {
		t.Error("failed to setup index1.tmpl")
		return
	}

	writer := NewBufferWriter()
	SetupLogger(LogLevelDebug, writer)

	funcMap := template.FuncMap{
		"add": func(a, b int32) int32 { return a + b },
	}

	tmplMgr := NewTemplateManager(testDir, ".tmpl", funcMap, time.Second)
	defer tmplMgr.Close()
	resultWriter := NewBufferWriter()
	model := &TestModel{"title", "message"}
	err = tmplMgr.Execute(resultWriter, "index", model)
	if err != nil {
		t.Error("failed to execute index", err)
		return
	}

	result := readBuffer(resultWriter.Buffer)
	if result != "titlemessage2" {
		t.Error("expected titlemessage2 but got", result)
		return
	}

	resultWriter = NewBufferWriter()
	err = tmplMgr.Execute(resultWriter, "index1", model)
	if err != nil {
		t.Error("failed to execute index1")
		return
	}

	result = readBuffer(resultWriter.Buffer)

	if result != "titlemessage" {
		t.Error("expected titlemessage but got", result)
		return
	}

	// test reload
	// don't change too quick
	time.Sleep(time.Millisecond * 50)
	err = ioutil.WriteFile(path.Join(testDir, "header.tmpl"), []byte("{{define \"header\"}}{{.}}updated{{end}}"), os.ModePerm)
	if err != nil {
		t.Error("failed to update header.tmpl")
		return
	}

	time.Sleep(time.Second * 2)
	resultWriter = NewBufferWriter()
	err = tmplMgr.Execute(resultWriter, "index", model)
	if err != nil {
		t.Error("failed to execute index")
		return
	}

	result = readBuffer(resultWriter.Buffer)
	if result != "titleupdatedmessage2" {
		t.Error("expected titleupdatedmessage2 but got", result)
		return
	}
}

func TestSkinManager(t *testing.T) {
	// test setup
	// create test folder
	testDir1, err := ioutil.TempDir("", "cytpltest")
	if err != nil {
		t.Error("failed to create test dir", err)
		return
	}

	defer os.RemoveAll(testDir1)

	// write template files
	err = ioutil.WriteFile(path.Join(testDir1, "header.tmpl"), []byte("{{define \"header\"}}defaultskin{{.}}{{end}}"), os.ModePerm)
	if err != nil {
		t.Error("failed to setup header.tmpl")
		return
	}

	err = ioutil.WriteFile(path.Join(testDir1, "index.tmpl"), []byte("{{define \"index\"}}{{template \"header\" .Title}}{{.Message}}{{end}}"), os.ModePerm)
	if err != nil {
		t.Error("failed to setup index.tmpl")
		return
	}

	SetupLogger(LogLevelDebug, &DummyWriter{})

	tmplMgr1 := NewTemplateManager(testDir1, ".tmpl", nil, time.Second)
	defer tmplMgr1.Close()

	// second skin
	// create test folder
	testDir2, err := ioutil.TempDir("", "cytpltest")
	if err != nil {
		t.Error("failed to create test dir", err)
		return
	}

	defer os.RemoveAll(testDir2)

	// write template files
	err = ioutil.WriteFile(path.Join(testDir2, "header.tmpl"), []byte("{{define \"header\"}}skin1{{.}}{{end}}"), os.ModePerm)
	if err != nil {
		t.Error("failed to setup header.tmpl")
		return
	}

	err = ioutil.WriteFile(path.Join(testDir2, "index.tmpl"), []byte("{{define \"index\"}}{{template \"header\" .Title}}{{.Message}}{{end}}"), os.ModePerm)
	if err != nil {
		t.Error("failed to setup index.tmpl")
		return
	}

	tmplMgr2 := NewTemplateManager(testDir2, ".tmpl", nil, time.Second)
	defer tmplMgr2.Close()

	skinMgr := NewSkinManager(tmplMgr1)
	skinMgr.AddSkin("skin1", tmplMgr2)

	resultWriter := NewBufferWriter()
	model := &TestModel{"title", "message"}
	err = skinMgr.GetDefaultSkin().Execute(resultWriter, "index", model)
	if err != nil {
		t.Error("failed to execute index")
		return
	}

	result := readBuffer(resultWriter.Buffer)
	if result != "defaultskintitlemessage" {
		t.Error("expected defaultskintitlemessage but got", result)
		return
	}

	resultWriter = NewBufferWriter()
	err = skinMgr.GetSkinOrDefault("skin1").Execute(resultWriter, "index", model)
	if err != nil {
		t.Error("failed to execute index")
		return
	}

	result = readBuffer(resultWriter.Buffer)

	if result != "skin1titlemessage" {
		t.Error("expected skin1titlemessage but got", result)
		return
	}

	resultWriter = NewBufferWriter()
	err = skinMgr.GetSkinOrDefault("skin2").Execute(resultWriter, "index", model)
	if err != nil {
		t.Error("failed to execute index")
		return
	}

	result = readBuffer(resultWriter.Buffer)

	if result != "defaultskintitlemessage" {
		t.Error("expected defaultskintitlemessage but got", result)
		return
	}

	m, ok := skinMgr.GetSkin("skin3")
	if ok {
		t.Error("unexpected skin3 in skin manager")
		return
	}

	m, ok = skinMgr.GetSkin("skin1")
	if !ok {
		t.Error("unexpected result, skin1 must exist")
		return
	}

	resultWriter = NewBufferWriter()
	err = m.Execute(resultWriter, "index", model)
	if err != nil {
		t.Error("failed to execute index")
		return
	}

	result = readBuffer(resultWriter.Buffer)

	if result != "skin1titlemessage" {
		t.Error("expected skin1titlemessage but got", result)
		return
	}
}
