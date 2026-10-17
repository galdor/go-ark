package template

import (
	"fmt"
	htmltemplate "html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	texttemplate "text/template"

	"go.n16f.net/ark/pkg/ark/utils"
)

func LoadTemplates(
	dirPath string, functions map[string]any,
) (*texttemplate.Template, *htmltemplate.Template, error) {
	textTemplate := texttemplate.New("")
	textTemplate.Option("missingkey=error")
	textTemplate.Funcs(Functions)
	textTemplate.Funcs(functions)

	htmlTemplate := htmltemplate.New("")
	htmlTemplate.Option("missingkey=error")
	htmlTemplate.Funcs(Functions)
	htmlTemplate.Funcs(functions)

	err := utils.WalkFS(dirPath,
		func(virtualPath, filePath string, info fs.FileInfo) error {
			isText := strings.HasSuffix(virtualPath, ".txt.gotpl")
			isHTML := strings.HasSuffix(virtualPath, ".html.gotpl")

			if !isText && !isHTML {
				return nil
			}

			relPath, err := filepath.Rel(dirPath, virtualPath)
			if err != nil {
				return fmt.Errorf("cannot compute relative path of %q: %w",
					virtualPath, err)
			}

			templateName := strings.TrimSuffix(filepath.ToSlash(relPath),
				".gotpl")

			templateData, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("cannot read %q: %w", filePath, err)
			}

			switch {
			case isText:
				template := textTemplate.New(templateName)
				_, err = template.Parse(string(templateData))

			case isHTML:
				template := htmlTemplate.New(templateName)
				_, err = template.Parse(string(templateData))
			}

			if err != nil {
				return fmt.Errorf("cannot parse %q: %w", filePath, err)
			}

			return nil
		})
	if err != nil {
		return nil, nil, err
	}

	return textTemplate, htmlTemplate, nil
}
