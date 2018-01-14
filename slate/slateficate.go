package slate

import (
	"os"
	"io"
	"github.com/growler/go-slate/slate/internal/slate"
	"sort"
	"strings"
	"fmt"
	"github.com/spf13/afero"
	"io/ioutil"
)

func makeTargetDirs(fs *afero.Afero, dirs ...string) error {
	for _, d := range dirs {
		if err := fs.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

// Configuration
type Params struct {
	MinifyHTML bool   // produce compact HTML
	MinifyJS   bool   // minify Javascript
	MinifyCSS  bool   // produce compact CSS
	StyleFile  string // load SCSS overrides
	LogoFile   string // use this logo (which should be located in images/ directory)
	Search     *bool  // if nil, use the default from index.html.md preamble
	RTL        *bool  // Right-to-Left CSS, if nil, use the default from index.html.md preamble
}

// Go Slate!
func Slateficate(src string, target *afero.Afero, params Params) error {
	var err error
	fs, err := slate.NewUnionFS(src)
	if err != nil {
		return err
	}
	err = makeTargetDirs(target, "javascripts", "stylesheets", "fonts", "images")
	if err != nil {
		return err
	}
	input, err := load(fs, params)
	if err != nil {
		return err
	}
	if err = input.produce(target, params.MinifyHTML); err != nil {
		return err
	}
	if err = copyJavaScripts(fs, target, input.Params.Search, params.MinifyJS); err != nil {
		return err
	}
	var styles []string
	if input.Params.Style != "" {
		styles = append(styles, input.Params.Style)
	}
	if params.StyleFile != "" {
		file, err := os.OpenFile(params.StyleFile, os.O_RDONLY, 0)
		if err != nil {
			return err
		}
		data, err := ioutil.ReadAll(file)
		file.Close()
		if err != nil {
			return err
		}
		styles = append(styles, string(data))
	}
	if err = copyStylesheetsAndFonts(fs, target, styles, input.Params.RTLEnabled, params.MinifyCSS); err != nil {
		return err
	}
	if err = copyImages(fs, target, input.Params.Logo); err != nil {
		return err
	}
	return nil
}

// Extract embedded slate components to target
func Extract(target string, overwrite bool, components ...string) error {
	var srcFiles []string
	var dstFiles = make(map[string]bool)
	var dst []string
	slate.FS().Walk("", func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			srcFiles = append(srcFiles, path)
		}
		return nil
	})
	sort.Strings(srcFiles)
	for _, c := range components {
		switch c {
		case "all":
			dst = srcFiles
			goto Copy
		case "contents":
			dstFiles["index.html.md"] = true
			dstFiles["includes/_errors.md"] = true
		case "fonts", "images", "layouts", "javascripts", "stylesheets":
			for _, s := range srcFiles {
				if strings.HasPrefix(s, c + "/") {
					dstFiles[s] = true
				}
			}
		default:
			if n := sort.SearchStrings(srcFiles, c); n < len(srcFiles) && srcFiles[n] == c {
				dstFiles[c] = true
			} else {
				return fmt.Errorf("unknown slate component or file %s", c)
			}
		}
	}
	for f := range dstFiles {
		dst = append(dst, f)
	}
Copy:
	if err := slate.CopyTo(target, 0640, overwrite, dst...); err != nil {
		return err
	}
	return nil
}

func ListBundled(w io.Writer) {
	var srcFiles []string
	slate.FS().Walk("", func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			srcFiles = append(srcFiles, path)
		}
		return nil
	})
	sort.Strings(srcFiles)
	for _, s := range srcFiles {
		w.Write([]byte(s))
		w.Write([]byte{'\n'})
	}
}