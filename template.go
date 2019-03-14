package cypress

import (
	"errors"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	//ErrNoFile no file given for Creating a template
	ErrNoFile = errors.New("No template file")

	//SkinDefault default skin name
	SkinDefault = "default"
)

type templateFileInfo struct {
	file        string
	lastModifed time.Time
}

// TemplateManager manages the templates by groups and update template group on-demand
// based on the template file update timestamp
type TemplateManager struct {
	lock      *sync.RWMutex
	root      *template.Template
	fileLock  *sync.RWMutex
	files     map[string]time.Time
	refresher *time.Ticker
	exitChan  chan bool
	funcs     template.FuncMap
}

// SkinSelector returns a skin name based on the request object
type SkinSelector interface {
	GetSkin(request *http.Request) string
}

// SkinSelectorFunc converts a function to SkinSelector interface
type SkinSelectorFunc func(*http.Request) string

// SkinManager a TemplateManager is mapped to a skin, skin manager manages TemplateManagers
// by names.
type SkinManager struct {
	defaultSkin *TemplateManager
	skins       map[string]*TemplateManager
	lock        *sync.RWMutex
	selector    SkinSelector
}

// NewTemplateManager creates a template manager for the given dir
func NewTemplateManager(dir, suffix string, funcs template.FuncMap, refreshInterval time.Duration) *TemplateManager {
	dirs := make([]string, 0, 10)
	tmplFiles := make([]string, 0, 20)
	filesTime := make(map[string]time.Time)

	// scan dir for all template files
	dirs = append(dirs, dir)
	for len(dirs) > 0 {
		current := dirs[0]
		dirs = dirs[1:]
		if !strings.HasSuffix(current, "/") {
			current = current + "/"
		}

		zap.L().Info("scan for template files", zap.String("dir", current), zap.String("suffix", suffix))
		files, err := ioutil.ReadDir(current)
		if err != nil {
			zap.L().Error("failed to scan directory for template files", zap.String("dir", current), zap.Error(err))
			continue
		}

		for _, file := range files {
			if file.IsDir() {
				dirs = append(dirs, current+file.Name()+"/")
			} else if strings.HasSuffix(file.Name(), suffix) {
				zap.L().Info("template file found", zap.String("file", file.Name()))
				path := current + file.Name()
				tmplFiles = append(tmplFiles, path)
				filesTime[path] = file.ModTime()
			}
		}
	}

	root := template.New("root")
	if funcs != nil {
		root.Funcs(funcs)
	}

	root, err := root.ParseFiles(tmplFiles...)
	if err != nil {
		zap.L().Error("failed parse files into root template, root will be defaulted to empty", zap.Error(err))
		root = template.New("empty")
	}

	mgr := &TemplateManager{
		lock:      &sync.RWMutex{},
		root:      root,
		fileLock:  &sync.RWMutex{},
		files:     filesTime,
		refresher: time.NewTicker(refreshInterval),
		exitChan:  make(chan bool),
		funcs:     funcs,
	}

	go func() {
		for {
			select {
			case <-mgr.refresher.C:
				mgr.refreshTemplates()
				break
			case <-mgr.exitChan:
				return
			}
		}
	}()
	return mgr
}

// Close closes the template manager and release all resources
func (manager *TemplateManager) Close() {
	manager.exitChan <- true
	manager.refresher.Stop()
	close(manager.exitChan)
}

// Execute execute the given template with the specified data and render the result to writer
func (manager *TemplateManager) Execute(writer io.Writer, name string, data interface{}) error {
	manager.lock.RLock()
	defer manager.lock.RUnlock()
	return manager.root.ExecuteTemplate(writer, name, data)
}

func (manager *TemplateManager) refreshTemplates() {
	files := make([]string, 0, len(manager.files))
	func() {
		manager.fileLock.RLock()
		defer manager.fileLock.RUnlock()
		for key := range manager.files {
			files = append(files, key)
		}
	}()

	for _, file := range files {
		var t time.Time
		var ok bool
		stat, err := os.Stat(file)
		if err != nil {
			zap.L().Error("unexpectedTmplRefreshError", zap.Error(err))
			continue
		}

		func() {
			manager.fileLock.RLock()
			defer manager.fileLock.RUnlock()
			t, ok = manager.files[file]
		}()

		if !ok {
			zap.L().Error("fileInfoBlockNotFound", zap.String("file", file))
			continue
		}

		if t.Before(stat.ModTime()) {
			root := template.New("root")
			if manager.funcs != nil {
				root.Funcs(manager.funcs)
			}

			root, err := root.ParseFiles(files...)
			if err != nil {
				zap.L().Error("failed to refresh template file", zap.String("file", file), zap.Error(err))
			} else {
				func() {
					manager.lock.Lock()
					defer manager.lock.Unlock()
					manager.root = root
				}()
			}
		}
	}
}

// NewSkinManager creates a skin manager object
func NewSkinManager(defaultSkin *TemplateManager) *SkinManager {
	return &SkinManager{defaultSkin, make(map[string]*TemplateManager), &sync.RWMutex{}, nil}
}

// AddSkin adds a skin
func (skinMgr *SkinManager) AddSkin(name string, tmplMgr *TemplateManager) {
	skinMgr.lock.Lock()
	defer skinMgr.lock.Unlock()
	skinMgr.skins[name] = tmplMgr
}

// RemoveSkin removes a skin
func (skinMgr *SkinManager) RemoveSkin(name string) {
	skinMgr.lock.Lock()
	defer skinMgr.lock.Unlock()
	delete(skinMgr.skins, name)
}

// GetDefaultSkin gets the default skin
func (skinMgr *SkinManager) GetDefaultSkin() *TemplateManager {
	skinMgr.lock.RLock()
	defer skinMgr.lock.RUnlock()
	return skinMgr.defaultSkin
}

// GetSkinOrDefault gets the skin with the given name if it's not
// found return the default skin
func (skinMgr *SkinManager) GetSkinOrDefault(name string) *TemplateManager {
	skinMgr.lock.RLock()
	defer skinMgr.lock.RUnlock()
	tmplMgr, ok := skinMgr.skins[name]
	if !ok {
		return skinMgr.defaultSkin
	}

	return tmplMgr
}

// GetSkin find skin by name
func (skinMgr *SkinManager) GetSkin(name string) (*TemplateManager, bool) {
	skinMgr.lock.RLock()
	defer skinMgr.lock.RUnlock()
	tmplMgr, ok := skinMgr.skins[name]
	return tmplMgr, ok
}

// ApplySelector find skin by applying selector, if selector is not configured
// default skin will be returned, if the skin name returned by the selector cannot
// be found, this returns nil
func (skinMgr *SkinManager) ApplySelector(request *http.Request) (*TemplateManager, string) {
	if skinMgr.selector == nil {
		return skinMgr.GetDefaultSkin(), SkinDefault
	}

	skinMgr.lock.RLock()
	defer skinMgr.lock.RUnlock()
	name := skinMgr.selector.GetSkin(request)
	skin, _ := skinMgr.GetSkin(name)
	return skin, name
}

// WithSelector sets the skin selector for the skin manager
func (skinMgr *SkinManager) WithSelector(selector SkinSelector) {
	skinMgr.lock.Lock()
	defer skinMgr.lock.Unlock()
	skinMgr.selector = selector
}
