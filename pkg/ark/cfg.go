package ark

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"text/template"

	"go.n16f.net/ark/pkg/ark/utils"
	"go.n16f.net/ark/pkg/ark/yaml"
)

func LoadCfg(filePath string, templateData, dest any) error {
	baseData, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("cannot read %q: %w", filePath, err)
	}

	data, err := RenderCfg(filePath, baseData, templateData)
	if err != nil {
		return err
	}

	opts := yaml.DecodingOptions{Strict: true}
	if err := yaml.DecodeWithOptions(data, dest, opts); err != nil {
		return fmt.Errorf("cannot parse %q: %w", filePath, err)
	}

	return nil
}

func RenderCfg(
	filePath string, templateContent []byte, templateData any,
) ([]byte, error) {
	tpl := template.New(filepath.Base(filePath))
	tpl.Option("missingkey=error")
	tpl.Funcs(utils.TemplateFunctions)

	if _, err := tpl.Parse(string(templateContent)); err != nil {
		return nil, fmt.Errorf("cannot parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, templateData); err != nil {
		return nil, fmt.Errorf("cannot execute template: %w", err)
	}

	return buf.Bytes(), nil
}
