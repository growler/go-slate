// Copyright 2017 Alexey Naidyonov. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.md file.

package main

import (
	"os"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/growler/go-slate/slate"
	"time"
	"github.com/spf13/afero"
	"path/filepath"
	"io/ioutil"
	"github.com/growler/go-imbed/imbed"
	"github.com/growler/go-slate/server"
	"errors"
)

var (
	cmd *cobra.Command
)

func setParams(params *slate.Params, opts generateOptions) error {
	params.MinifyHTML = true
	params.MinifyCSS = true
	params.MinifyJS = true
	if opts.search && opts.noSearch {
		return errors.New("both --search and --no-search set")
	} else if opts.search {
		params.Search = new(bool)
		*params.Search = true
	} else if opts.noSearch {
		params.Search = new(bool)
	}
	if opts.rtl && opts.noRtl {
		return errors.New("both --rtl and --no-rtl set")
	} else if opts.rtl {
		params.RTL = new(bool)
		*params.RTL = true
	} else if opts.noRtl {
		params.RTL = new(bool)
	}
	params.LogoFile = opts.logoFile
	params.StyleFile = opts.styleFile
	for _, s := range opts.noMinify {
		switch s {
		case "all":
			params.MinifyCSS = false
			params.MinifyJS = false
			params.MinifyHTML = false
			return nil
		case "css":
			params.MinifyCSS = false
		case "js":
			params.MinifyJS = false
		case "html":
			params.MinifyHTML = false
		default:
			return fmt.Errorf("unknown parameter %s to --no-minify", s)
		}
	}
	return nil
}

func cmdVersion() *cobra.Command {
	cmd := &cobra.Command{
		Use: "version",
		Short: "prints version",
		Long: `Prints versions of both go-slate and embedded Slate`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("go-slate version: %s\n", slate.GoSlateVersion)
			fmt.Printf("slate version:    %s\n", slate.SlateVersion)
			return nil
		},
	}
	return cmd
}

type generateOptions struct {
	noMinify []string
	search    bool
	noSearch  bool
	rtl       bool
	noRtl     bool
	styleFile string
	logoFile  string
}

func genOpts(cmd *cobra.Command, opts *generateOptions) {
	cmd.Flags().BoolVar(&opts.rtl, "rtl", false, "enable right-to-left scripts support (overrides option in source file)")
	cmd.Flags().BoolVar(&opts.noRtl, "no-rtl", false, "disable right-to-left scripts support (overrides option in source file)")
	cmd.Flags().StringVarP(&opts.styleFile, "style", "s", "", "supply an SCSS `file` to adjust documentation styles (overrides option in source file)")
	cmd.Flags().StringVarP(&opts.logoFile, "logo", "l", "", "supply a logo image `file` to use (overrides option in source file)")
	cmd.Flags().BoolVar(&opts.noSearch, "search", false, "enable Slate search block (overrides option in source file)")
	cmd.Flags().BoolVar(&opts.noSearch, "no-search", false, "disable Slate search block (overrides option in source file)")
	cmd.Flags().StringSliceVar(&opts.noMinify, "no-minify", []string{}, "disable compaction, comma-separated list of `css|js|html|all`")
}

func cmdSite() *cobra.Command {
	var opts generateOptions
	cmd := &cobra.Command{
		Use: "site [source directory] [output directory]",
		Short: "renders documentation from source directory to output directory",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var params slate.Params
			if err := setParams(&params, opts); err != nil {
				return err
			}
			p, err := filepath.Abs(args[1])
			if err != nil {
				return err
			}
			fs := &afero.Afero{Fs: afero.NewBasePathFs(afero.NewOsFs(), p)}
			if err := slate.Slateficate(args[0], fs, params); err != nil {
				return err
			}
			return nil
		},
	}
	genOpts(cmd, &opts)
	return cmd
}

func rmtree(name string) {
	var files []string
	var dirs []string
	filepath.Walk(name, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			dirs = append(dirs, path)
		} else {
			files = append(files, path)
		}
		return nil
	})
	for j := len(files) - 1; j >= 0; j-- {
		os.Remove(files[j])
	}
	for j := len(dirs) - 1; j >= 0; j-- {
		os.Remove(dirs[j])
	}
}

func cmdPackage() *cobra.Command {
	var opts generateOptions
	var workDir string
	var pkgName string
	cmd := &cobra.Command{
		Use: "package [source directory] [resulting package directory]",
		Short: "produces an embeddable package with rendered documentation content and HTTP handler",
		Long: `
Produces an embeddable package with rendered documentation content using github.com/growler/go-imbed tool.
The package will also contain HTTP handler to serve content with standard Go http server.
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var params slate.Params
			if err := setParams(&params, opts); err != nil {
				return err
			}
			if workDir == "" {
				tmpDir, err := ioutil.TempDir(os.TempDir(), ".go-slate")
				if err != nil {
					return fmt.Errorf("error creating temp directory: %s", err)
				}
				defer rmtree(tmpDir)
				workDir = tmpDir
			}
			fs := &afero.Afero{Fs: afero.NewBasePathFs(afero.NewOsFs(), workDir)}
			if err := slate.Slateficate(args[0], fs, params); err != nil {
				return err
			}
			if pkgName == "" {
				pkgName = filepath.Base(args[1])
			}
			if err := imbed.Imbed(workDir, args[1], pkgName, imbed.CompressAssets | imbed.BuildHttpHandlerAPI); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&pkgName, "pkg", "n", "", "use this package `name` (otherwise the base name of resulting package directory will be used)")
	cmd.Flags().StringVarP(&workDir, "work-dir", "d", "", "use specified working `directory` to render documentation results")
	genOpts(cmd, &opts)
	return cmd
}

func cmdServer() *cobra.Command {
	var opts generateOptions
	var tlsCert string
	var tlsKey string
	var monitor bool
	cmd := &cobra.Command{
		Use: "server [source directory] [address to listen at]",
		Short: "serves rendered API documentation over HTTP(S)",
		Long: `
Serves API documentation over HTTP(S) at specified address. Monitors changes if requested and
updates documentation in real time.
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var params slate.Params
			if err := setParams(&params, opts); err != nil {
				return err
			}
			return server.Serve(args[0], params, args[1], tlsCert, tlsKey, monitor)
		},
	}
	cmd.Flags().StringVarP(&tlsCert, "tls-cert", "c", "", "TLS certificate `file` to use")
	cmd.Flags().StringVarP(&tlsKey, "tls-key", "k", "", "TLS key `file` to use")
	cmd.Flags().BoolVarP(&monitor, "monitor-changes", "m", false, "monitor changes and re-generate documentation if necessary")
	genOpts(cmd, &opts)
	return cmd
}

func cmdExtract() *cobra.Command {
	var (
		list bool
		overwrite bool
	)
	cmd := &cobra.Command{
		Use: "extract [directory] [components to extract...]",

		Short: "extracts slate files bundled with go-slate to specified directory",

		Long: `
Extracts Slate files bundled with go-slate to a specified directory.
By default, extracts only slate example documentation project content (Markdown files).
Otherwise, a specified list of components will be extracted. Specify word "all" to extract
all files, any of "contents", "layouts", "stylesheets", "javascripts", "images" to extract
specific kinds, or a single file name:

$ go-slate extract -l
...
$ go-slate extract . contents stylesheets
...
edit index.html.md
edit stylesheets/_variables.scss
...
$ go-slate package . internal/apidoc
$ 
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if list {
				slate.ListBundled(os.Stdout)
				return nil
			} else if len(args) == 0 {
				return fmt.Errorf("")
			} else if len(args) == 1 {
				return slate.Extract(args[0], overwrite,"contents")
			} else {
				return slate.Extract(args[0], overwrite, args[1:]...)
			}
		},
	}
	cmd.Flags().BoolVarP(&list, "list", "l", false, "lists slate components available to extract")
	cmd.Flags().BoolVarP(&overwrite, "overwrite", "w", false, "overwrite existing files")
	return cmd
}

func init() {
	var timings bool
	var startTs time.Time
	cmd = &cobra.Command{
		Use: "go-slate",
		Short: "a simple Go tool to generate API documentation using brilliant github.com/lord/slate layout",
		Long: `
go-slate is a CLI tool to generate API documentation using brilliant Slate layout 
by Robert Lord. go-slate contains bundled Slate ` + slate.SlateVersion + ` and requires no additional
software to install. go-slate can also generate a go package with embedded 
documentation to include into Go binary. go-slate uses bundled source files or files from 
the filesystem if present. A simple HTTP(s) server to serve rendered documentation is 
also provided.

Assuming following source tree

    src
    └── service
        ├── main.go
        └── apidoc

the typical usage would be:

1. Extract template content

    $ cd src/service
    $ go-slate extract apidoc contents

2. Now edit apidoc/index.html.md and add following lines to 

    // go:generate go-slate package apidoc internal/apidoc
    package main

    import (
        "net/http"
        "fmt"
        "service/internal/apidoc"
    )

    func main() {
        http.HandleFunc("/help/", apidoc.HTTPHandlerWithPrefix("/help"))
        //...
        if err := http.ListenAndServe(":8080", nil); err != nil{
            fmt.Println(err)
        }
    }

3. Generate package with embedded documentation

    $ go generate service
    $ go build service

3. Enjoy beautiful documentation at http://service:8080/help/
`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if timings {
				startTs = time.Now()
			}
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if timings {
				executionTime := time.Now().Sub(startTs)
				fmt.Printf("%0.3fs\n", executionTime.Seconds())
			}
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(
		cmdVersion(),
		cmdSite(),
		cmdPackage(),
		cmdExtract(),
		cmdServer(),
	)
	cmd.PersistentFlags().BoolVarP(&timings, "time", "t", false, "prints command execution time")
}

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
