package slate

//go:generate go-imbed --no-http-handler --fs --union-fs _slate internal/slate

import (
	"bufio"
	"bytes"
	"regexp"

	"github.com/growler/go-slate/slate/internal/slate"
	"github.com/tdewolff/minify"
	"github.com/wellington/go-libsass"
	"fmt"
	"strings"
	"path/filepath"
	"errors"
	"io"
	"io/ioutil"
	"context"
	"github.com/spf13/afero"
	"github.com/tdewolff/minify/js"
	"net/url"
	"path"
)

const GoSlateVersion = "v1.0.0"
const SlateVersion = "v2.1.0"

func copyJavaScripts(fs slate.FileSystem, target *afero.Afero, search bool, minifyJs bool) error {
	var files []string
	jsDir, err := fs.Open("javascripts")
	if err != nil {
		return err
	}
	defer jsDir.Close()
	jsFiles, err := jsDir.Readdir(-1)
	if err != nil {
		return err
	}
	for i := range jsFiles {
		if !jsFiles[i].IsDir() && strings.HasSuffix(jsFiles[i].Name(), ".js") {
			files = append(files, jsFiles[i].Name())
		}
	}
	for _, jsFile := range files {
		if jsFile == "all.js" && !search {
			continue
		} else if jsFile == "all_nosearch.js" && search {
			continue
		}
		if err := produceJavaScript(fs, jsFile, target, minifyJs); err != nil {
			return err
		}
	}
	return nil
}

var jsReqRE = regexp.MustCompile(`^//= require (.+)$`)

func produceJavaScript(fs slate.FileSystem, src string, target *afero.Afero, minifyJs bool) error {
	var buf bytes.Buffer
	var loaded []string
	var err error
	if err := loadJS(fs, src, &buf, &loaded); err != nil {
		return err
	}
	var result []byte
	if minifyJs {
		m := minify.New()
		m.AddFunc("text/javascript", js.Minify)
		result, err = m.Bytes("text/javascript", buf.Bytes())
		if err != nil {
			return err
		}
	} else {
		result = buf.Bytes()
	}
	file, err := target.TempFile("javascripts", ".slate")
	fileName := filepath.Join("javascripts", filepath.Base(file.Name()))
	if err != nil {
		return err
	}
	defer func() {
		file.Close()
		target.Remove(fileName)
	}()
	if _, err = file.Write(result); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	} else {
		return target.Rename(fileName, filepath.Join("javascripts", src))
	}
}

func loadJS(fs slate.FileSystem, source string, buffer *bytes.Buffer, loaded *[]string) error {
	for _, s := range *loaded {
		if s == source {
			return nil
		}
	}
	*loaded = append(*loaded, source)
	inp, err := fs.Open(path.Join("javascripts", source))
	if err != nil {
		return err
	}
	defer inp.Close()
	dir := path.Dir(source)
	rdr := bufio.NewScanner(inp)
	flg := true
	for rdr.Scan() {
		line := rdr.Text()
		if flg {
			m := jsReqRE.FindAllStringSubmatch(rdr.Text(), -1)
			if len(m) > 0 {
				required := filepath.Clean(filepath.Join(dir, m[0][1]) + ".js")
				if err != nil {
					return err
				}
				err = loadJS(fs, required, buffer, loaded)
				if err != nil {
					return err
				}
				continue
			} else {
				flg = false
			}
		}
		buffer.WriteString(line)
		buffer.WriteByte('\n')
	}
	return err
}

func updateTarget(name string, fs slate.FileSystem, target *afero.Afero) error {
	fi, err := fs.Stat(name)
	if err != nil {
		return err
	}
	dfi, err := target.Stat(name)
	if err == nil {
		if fi.Size() == dfi.Size() && fi.ModTime() == dfi.ModTime() {
			return nil
		}
	}
	dst, err := target.TempFile(path.Dir(name), ".image")
	dstName := filepath.Join(path.Dir(name), filepath.Base(dst.Name()))
	if err != nil {
		return err
	}
	defer func() {
		dst.Close()
		target.Remove(dstName)
	}()
	src, err := fs.Open(name)
	if err != nil {
		return err
	}
	defer src.Close()
	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}
	return target.Rename(dstName, name)
}

func copyImages(fs slate.FileSystem, target *afero.Afero, logo string) error {
	imagesDir, err := fs.Open("images")
	if err != nil {
		return fmt.Errorf("error copying images: %s", err)
	}
	imagesFiles, err := imagesDir.Readdir(-1)
	if err != nil {
		return fmt.Errorf("error copying images: %s", err)
	}
	for _, fi := range imagesFiles {
		if fi.IsDir() || (fi.Name() == "logo.png" && logo != "" && logo != fi.Name()) {
			continue
		}
		err := updateTarget(path.Join("images", fi.Name()), fs, target)
		if err != nil {
			return fmt.Errorf("error copying image file %s: %s", fi.Name(), err)
		}
	}
	return nil
}

func copyStylesheetsAndFonts(fs slate.FileSystem, target *afero.Afero, style []string, rtlEnabled bool, minifyCss bool) error {
	var fonts = make(map[string]bool)
	for _, s := range style {
		libsass.RegisterHeader(s)
	}
	libsass.RegisterSassFunc("font-url($url)", func(ctx context.Context, in libsass.SassValue) (*libsass.SassValue, error) {
		var args []string
		err := libsass.Unmarshal(in, &args)
		if err != nil {
			return nil, err
		}
		if len(args) != 1 {
			return nil, errors.New("invalid font-url call")
		}
		url, err := url.Parse(args[0])
		if err != nil {
			return nil, fmt.Errorf("illegal font url font-url(%s)", args[0])
		}
		fonts[url.Path] = true
		ret, err := libsass.Marshal(fmt.Sprintf("url(../fonts/%s)", args[0]))
		if err != nil {
			return nil, err
		}
		return &ret, nil
	})
	var targets []string
	imports := libsass.NewImports()
	imports.Init()
	styleDir, err := fs.Open("stylesheets")
	if err != nil {
		return fmt.Errorf("can't open Slate stylesheets directory: %s", err)
	}
	styleFiles, err := styleDir.Readdir(-1)
	if err != nil {
		return fmt.Errorf("can't list Slate stylesheets directory: %s", err)
	}
	if !rtlEnabled {
		imports.Add("", "rtl", []byte{})
	}
	for _, fi := range styleFiles {
		s := fi.Name()
		if fi.IsDir() {
			continue
		}
		if strings.HasPrefix(s, "_") && strings.HasSuffix(s, ".scss") {
			if !rtlEnabled && s == "_rtl.scss" {
				continue
			}
			file, err := fs.Open(path.Join("stylesheets", s))
			if err != nil {
				return fmt.Errorf("can't open Slate stylesheets file %s %s", s, err)
			}
			data, err := ioutil.ReadAll(file)
			file.Close()
			if err != nil {
				return err
			}
			imports.Add("", s[1:len(s)-5], data)
		} else if strings.HasSuffix(s, ".css.scss") {
			targets = append(targets, s[:len(s)-5])
		}
	}
	for _, s := range targets {
		err := func () error {
			src, err := fs.Open(path.Join("stylesheets", s+".scss"))
			if err != nil {
				return fmt.Errorf("can't open Slate stylesheet source file %s: %s", s+".scss", err)
			}
			defer src.Close()
			dst, err := target.TempFile("stylesheets", ".scss")
			dstName := filepath.Join("stylesheets", filepath.Base(dst.Name()))
			if err != nil {
				return err
			}
			defer func() {
				dst.Close()
				target.Remove(dstName)
			}()
			var style int
			if minifyCss {
				style = libsass.COMPRESSED_STYLE
			} else {
				style = libsass.EXPANDED_STYLE
			}
			compiler, err := libsass.New(dst, src,
				libsass.ImportsOption(imports),
				libsass.FontDir("fonts"),
				libsass.ImgDir("images"),
				libsass.OutputStyle(style),
			)
			if err != nil {
				return err
			}
			if err = compiler.Run(); err != nil {
				return err
			}
			src.Close()
			dst.Close()
			return target.Rename(dstName, filepath.Join("stylesheets", s))
		}()
		if err != nil {
			return err
		}
	}
	for k := range fonts {
		err := updateTarget(path.Join("fonts", k), fs, target)
		if err != nil {
			return fmt.Errorf("error copying font %s: %s", k, err)
		}
	}
	return nil
}
