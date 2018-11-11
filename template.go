package cypress

import (
	"errors"
	"html/template"
	"net/http"
	"os"
	"path"
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

type templateInfo struct {
	files []string
	tmpl  *template.Template
}

type templateFileInfo struct {
	file        string
	references  []*templateInfo
	lastModifed time.Time
	lock        *sync.RWMutex
}

// TemplateManager manages the templates by groups and update template group on-demand
// based on the template file update timestamp
type TemplateManager struct {
	dir       string
	lock      *sync.RWMutex
	templates map[string]*template.Template
	fileLock  *sync.RWMutex
	files     map[string]*templateFileInfo
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
func NewTemplateManager(dir string, refreshInterval time.Duration) *TemplateManager {
	mgr := &TemplateManager{
		dir:       dir,
		lock:      &sync.RWMutex{},
		templates: make(map[string]*template.Template),
		fileLock:  &sync.RWMutex{},
		files:     make(map[string]*templateFileInfo),
		refresher: time.NewTicker(refreshInterval),
		exitChan:  make(chan bool),
		funcs:     nil,
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

// Funcs add a funcMap to TemplateManager
func (manager *TemplateManager) Funcs(funcMap template.FuncMap) *TemplateManager {
	manager.funcs = funcMap
	return manager
}

// GetOrCreateTemplate gets a template from cache or create a new template
// if no cache found
func (manager *TemplateManager) GetOrCreateTemplate(files ...string) (*template.Template, error) {
	if len(files) == 0 {
		return nil, ErrNoFile
	}

	name := path.Base(files[0])
	manager.lock.RLock()
	tmpl, ok := manager.templates[name]
	manager.lock.RUnlock()
	if ok {
		zap.L().Debug("templateCacheHit", zap.String("name", name))
		return tmpl, nil
	}

	resolvedFiles := make([]string, len(files))
	for index, file := range files {
		resolvedFiles[index] = path.Join(manager.dir, file)
	}

	tmpl = template.New(name).Funcs(manager.funcs)
	tmpl, err := tmpl.ParseFiles(resolvedFiles...)
	if err != nil {
		return nil, err
	}

	manager.lock.Lock()
	manager.templates[name] = tmpl
	manager.lock.Unlock()

	for _, resolvedFile := range resolvedFiles {
		manager.fileLock.RLock()
		fileInfo, ok := manager.files[resolvedFile]
		manager.fileLock.RUnlock()

		if !ok {
			stat, err := os.Stat(resolvedFile)
			if err != nil {
				zap.L().Error("unexpectedStatError", zap.Error(err))
				return tmpl, nil
			}

			fileInfo = &templateFileInfo{
				file:        resolvedFile,
				references:  make([]*templateInfo, 0, 10),
				lastModifed: stat.ModTime(),
				lock:        &sync.RWMutex{},
			}

			manager.fileLock.Lock()
			manager.files[resolvedFile] = fileInfo
			manager.fileLock.Unlock()
		}

		fileInfo.lock.Lock()
		fileInfo.references = append(fileInfo.references, &templateInfo{
			files: resolvedFiles,
			tmpl:  tmpl,
		})
		fileInfo.lock.Unlock()
	}

	return tmpl, nil
}

func (manager *TemplateManager) refreshTemplates() {
	manager.fileLock.RLock()
	files := make([]string, 0, len(manager.files))
	for key := range manager.files {
		files = append(files, key)
	}

	manager.fileLock.RUnlock()

	for _, file := range files {
		stat, err := os.Stat(file)
		if err != nil {
			zap.L().Error("unexpectedTmplRefreshError", zap.Error(err))
			continue
		}

		manager.fileLock.RLock()
		fileInfo, ok := manager.files[file]
		manager.fileLock.RUnlock()

		if !ok {
			zap.L().Error("fileInfoBlockNotFound", zap.String("file", file))
			continue
		}

		if fileInfo.lastModifed.Before(stat.ModTime()) {
			// reduce the lock time, we sacrifies the memory
			fileInfo.lock.RLock()
			refs := make([]*templateInfo, len(fileInfo.references))
			copy(refs, fileInfo.references)
			fileInfo.lock.RUnlock()

			for _, ref := range refs {
				tmplName := path.Base(ref.files[0])
				tmpl := template.New(tmplName).Funcs(manager.funcs)
				tmpl, err := tmpl.ParseFiles(ref.files...)
				if err != nil {
					zap.L().Error("failedToRefreshTemplate", zap.String("template", tmplName), zap.String("file", file), zap.Error(err))
					continue
				}

				manager.lock.Lock()
				manager.templates[tmplName] = tmpl
				manager.lock.Unlock()

				fileInfo.lastModifed = stat.ModTime()
				zap.L().Info("templateRefreshed", zap.String("template", tmplName), zap.String("file", file))
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
