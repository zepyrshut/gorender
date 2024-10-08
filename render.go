package gorender

import (
	"bytes"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/justinas/nosurf"
)

type TemplateCache map[string]*template.Template

type Render struct {
	EnableCache bool
	// TemplatesPath es la ruta donde se encuentran las plantillas de la
	// aplicación, pueden ser bases, fragmentos o ambos. Lo que quieras.
	TemplatesPath string
	// PageTemplatesPath es la ruta donde se encuentran las plantillas de las
	// páginas de la aplicación. Estas son las que van a ser llamadas para
	// mostrar en pantalla.
	PageTemplatesPath string
	TemplateCache     TemplateCache
	Functions         template.FuncMap
}

type OptionFunc func(*Render)

type TemplateData struct {
	Data map[string]interface{}
	// SessionData contiene los datos de la sesión del usuario.
	SessionData interface{}
	// FeedbackData tiene como función mostrar los mensajes habituales de
	// información, advertencia, éxito y error. No va implícitamente relacionado
	// con los errores de validación de formularios pero pueden ser usados para
	// ello.
	FeedbackData map[string]string
	// FormData es una estructura que contiene los errores de validación de los
	// formularios además de los valores que se han introducido en los campos.
	FormData  FormData
	CSRFToken string
	Page      Pages
}

func WithRenderOptions(opts *Render) OptionFunc {
	return func(re *Render) {
		re.TemplatesPath = opts.TemplatesPath
		re.PageTemplatesPath = opts.PageTemplatesPath

		if opts.Functions != nil {
			for k, v := range opts.Functions {
				re.Functions[k] = v
			}
		}

		if opts.EnableCache {
			re.EnableCache = opts.EnableCache
			re.TemplateCache, _ = re.createTemplateCache()
		}
	}
}

func New(opts ...OptionFunc) *Render {
	functions := template.FuncMap{
		"translateKey":   translateKey,
		"or":             or,
		"containsErrors": containsErrors,
	}

	config := &Render{
		EnableCache:       false,
		TemplatesPath:     "templates",
		PageTemplatesPath: "templates/pages",
		TemplateCache:     TemplateCache{},
		Functions:         functions,
	}

	return config.apply(opts...)
}

func (re *Render) apply(opts ...OptionFunc) *Render {
	for _, opt := range opts {
		opt(re)
	}

	return re
}

func addDefaultData(td *TemplateData, r *http.Request) *TemplateData {
	td.CSRFToken = nosurf.Token(r)
	return td
}

func (re *Render) Template(w http.ResponseWriter, r *http.Request, tmpl string, td *TemplateData) error {
	var tc TemplateCache
	var err error

	if re.EnableCache {
		tc = re.TemplateCache
	} else {
		tc, err = re.createTemplateCache()
		if err != nil {
			slog.Error("error creating template cache:", "error", err)
			return err
		}
	}

	t, ok := tc[tmpl]
	if !ok {
		return errors.New("can't get template from cache")
	}

	buf := new(bytes.Buffer)
	td = addDefaultData(td, r)
	err = t.Execute(buf, td)
	if err != nil {
		slog.Error("error executing template:", "error", err)
		return err
	}

	_, err = buf.WriteTo(w)
	if err != nil {
		slog.Error("error writing template to browser:", "error", err)
	}

	return nil
}

func findHTMLFiles(root string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && filepath.Ext(path) == ".html" {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

func (re *Render) createTemplateCache() (TemplateCache, error) {
	myCache := TemplateCache{}

	pagesTemplates, err := findHTMLFiles(re.PageTemplatesPath)
	if err != nil {
		return myCache, err
	}

	files, err := findHTMLFiles(re.TemplatesPath)
	if err != nil {
		return myCache, err
	}

	for function := range re.Functions {
		slog.Info("function found", "function", function)
	}

	for _, file := range pagesTemplates {
		name := filepath.Base(file)
		ts, err := template.New(name).Funcs(re.Functions).ParseFiles(append(files, file)...)
		if err != nil {
			return myCache, err
		}

		myCache[name] = ts
	}

	return myCache, nil
}
