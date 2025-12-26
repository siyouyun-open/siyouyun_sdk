package localize

import (
	"io/fs"
	"log"
	"os"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/i18n"
)

var Instance *ControllerInstance

type ControllerInstance struct {
	holder      *i18n.I18n
	defaultLang string
}

// NewInstance init i18n instance
func NewInstance() {
	Instance = &ControllerInstance{
		holder: i18n.New(),
	}
	Instance.holder.DefaultMessageFunc = func(langInput, langMatched, key string, args ...interface{}) string {
		log.Printf("[WARN] I18n key not exist: langInput=%s langMatched=%s key=%s\n", langInput, langMatched, key)
		return key
	}
}

// LoadFS is a method shortcut to load files using
// an `embed.FS` or `fs.FS` or `http.FileSystem` value.
// The "pattern" is a classic glob pattern.
func (c *ControllerInstance) LoadFS(fileSystem fs.FS, pattern string, languages ...string) error {
	err := c.holder.LoadFS(fileSystem, pattern, languages...)
	if err != nil {
		return err
	}
	c.setDefaultLangByEnv()
	return nil
}

// Load is a method shortcut to load files using a filepath.Glob pattern.
// It returns a non-nil error on failure.
func (c *ControllerInstance) Load(globPattern string, languages ...string) error {
	err := c.holder.Load(globPattern, languages...)
	if err != nil {
		return err
	}
	c.setDefaultLangByEnv()
	return nil
}

// ConfigIris config iris i18n
func (c *ControllerInstance) ConfigIris(app *iris.Application) {
	if c.holder != nil {
		app.I18n = c.holder
	}
}

// Tr returns a translated message based on the "lang" language code
// and its key with any optional arguments attached to it.
func (c *ControllerInstance) Tr(lang string, key string, args ...interface{}) string {
	if c.holder == nil {
		return key
	}
	return c.holder.Tr(lang, key, args...)
}

// TrDefault returns a translated message based on the default language code
// and its key with any optional arguments attached to it.
func (c *ControllerInstance) TrDefault(key string, args ...interface{}) string {
	if c.holder == nil {
		return key
	}
	return c.holder.Tr(c.defaultLang, key, args...)
}

// GetDefaultLang get default lang
func (c *ControllerInstance) GetDefaultLang() string {
	return c.defaultLang
}

// setDefaultLangByLocale set default lang by env
func (c *ControllerInstance) setDefaultLangByEnv() {
	c.defaultLang = os.Getenv("APP_LANG")
	if c.defaultLang == "" {
		c.defaultLang = "en-US"
	}
	if !c.holder.SetDefault(c.defaultLang) {
		c.defaultLang = c.holder.Tags()[0].String()
	}
}
