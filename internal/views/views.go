package views

import (
	"embed"
	"errors"
	"html/template"
	"io"
	"log/slog"
)

var (
	ErrViewNotFound    = errors.New("view not found")
	ErrTemplateParse   = errors.New("failed to parse template")
	ErrTemplateExecute = errors.New("failed to execute template")
)

//go:embed templates/*
var views embed.FS

var viewMap = map[string]string{
	"public:landing":    "templates/public/landing.html",
	"public:login":      "templates/public/login.html",
	"private:dashboard": "templates/private/dashboard.html",
}

type ViewManager struct {
	logger *slog.Logger
}

func NewViewManager(
	logger *slog.Logger) *ViewManager {
	return &ViewManager{
		logger: logger,
	}
}

func (v *ViewManager) RenderLandingPage(lpd LandingPageData, w io.Writer) error {
	return v.renderPageView("public:landing", lpd.toData(), w)
}

func (v *ViewManager) RenderLoginPage(lpd LoginPageData, w io.Writer) error {
	return v.renderPageView("public:login", lpd.toData(), w)
}

func (v *ViewManager) RenderDashboardPage(dpd UserDashboardPageData, w io.Writer) error {
	return v.renderPageView("private:dashboard", dpd.toData(), w)
}

func (v *ViewManager) renderPageView(name string, data map[string]any, w io.Writer) error {
	return v.renderView(name, data, w)
}

func (v *ViewManager) renderView(name string, data map[string]any, w io.Writer) error {
	view, ok := viewMap[name]
	if !ok {
		return ErrViewNotFound
	}
	tmpl, err := template.ParseFS(views, "templates/base.html", view)
	if err != nil {
		return ErrTemplateParse
	}
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		return ErrTemplateExecute
	}
	return nil
}
