# go-slate

go-slate is a CLI tool to generate API documentation using brilliant [Slate](https://github.com/lord/slate) layout 
by [Robert Lord](https://github.com/lord). go-slate contains bundled Slate (including Kittn API Example Documentation) 
and requires no additional software to install.

go-slate can also generate a Go package with embedded documentation to include into Go binary. 
A simple HTTP(S) server to serve rendered documentation is also provided.

## Why

Slate is arguably the best tool around to quickly hack an API documentation which
will look really good. However it uses Ruby `bundler` and brings loads of software
which did not go well with my Mac. Besides, I really got used to Go _all you need is
a single binary_ way of things. 

There were also [Hugo DocuAPI theme](https://github.com/bep/docuapi) (based on Slate, too), 
but it has its own drawbacks, mostly because of different ways middleman and backfriday render 
contents.  

So I quickly hacked go-slate.

## Installation

```text
$ go get -u github.com/growler/go-slate
```

## How to start

Assuming following source tree

    src
    └── service
        ├── main.go
        └── apidoc

the typical usage would be:

1. Extract template content

```bash
go-slate extract src/service/apidoc contents
```

2. Now edit `apidoc/index.html.md` and add following lines to your `main.go`:

```go
//go:generate go-slate package apidoc internal/apidoc

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
```

3. Generate package with embedded documentation

```bash
go generate service
go build service
```

3. Enjoy beautiful documentation at `http://service:8080/help/`

## Usage

```bash
go-slate [command]
```

Available commands:

    help        Help about any command
    extract     extracts slate files bundled with go-slate to specified directory
    package     produces an embeddable package with rendered documentation content and HTTP handler
    site        renders documentation from source directory to output directory
    server      serves rendered API documentation over HTTP(S)
    version     prints version

## Extact 

```bash
go slate extract [directory] [components to extract...]
```

Extracts Slate files bundled with go-slate to a specified directory.
With none components supplied, extracts only slate example documentation 
project content (Markdown files). Otherwise, a specified list of components will be 
extracted. The component name might be either file name or one of

- all
- contents
- layouts
- stylesheets
- javascripts
- images

To list bundled Slate contents, use `go slate extract -l`

## Rendering commands options

Commands `site`, `package` and `server` render content and share a set of options

`--no-minify css|jss|html|all`

Disables compact output for a specific type of content. Multiple options can be listed with comma.

`--logo`

Defines a logo file to use, overriding default or setting in [document preamble](#slate-preamble-options). 
Use word `none` to disable logo block completely.

`--rtl` and `--no-rtl`

Enables or disable Right-to-left scripts support, overriding default (no) or setting in [document preamble](#slate-preamble-options). 

`--search` and `--no-search`

Enables or disable Slate search block, overriding setting in [document preamble](#slate-preamble-options). 

`--style file`

Loads an SCSS style to adjust Slate styles. Note that this adds to, and not override [document preamble](#slate-preamble-options)
option `style`.  A list of available variables can be obtained by `go-slate extract . stylesheets/_variables.scss`

## Site

```bash
go-slate site [source directory] [output directory] [flags]
```

Renders documentation to a destination.

## Package

```bash
go-slate package [source directory] [resulting package directory] [flags]
```

Produces an embeddable Go package using [go-imbed](https://github.com/growler/go-imbed)

`--pkg name`

By default, `package` takes package name from the last path item of resulting package directory. This option
sets package name explicitly.

`--work-dir directory`

By default, `package` renders content in a temporary directory and removes it afterwards. This option tells
`go-slate` to use specified directory and keep content afterwards.

## Server

```bash
go-slate server [source directory] [address to listen at] [flags]
```

Starts an HTTP server listening at the specified address and serving rendered content.

`--tls-cert file` and `--tls-key file`

If both options are specified, server will serve HTTPS.

`--monitor-changes`

Monitor changes of source content and re-render if necessary. Any rendering error will be printed
to stdout.

(A neat trick: `go-slate server <empty directory> :8080` will serve the Slate example Kittn API Documentation)

## Slate preamble options

`go-slate` supports Slate preamble options:

```yaml
# document title
title: API Reference 

# must be one of supported by https://github.com/alecthomas/chroma
language_tabs: 
  - shell
  - ruby
  - python
  - javascript

# a list of footers to place under TOC navigation menu
toc_footers:  
  - <a href='#'>Sign Up for a Developer Key</a>
  - <a href='https://github.com/lord/slate'>Documentation Powered by Slate</a>

# a list of files to include from includes directory.
# files must be named as includes/_<name>.md
includes:
  - errors

# enable search block
search: true 
```

In addition, `go-slate` defines a few others:

```yaml
# a logo file to use. file must be located 
# in directory images. Use `none` to disable logo
logo: logo.png    

# enable RTL support
enable_rtl: true  

# use code highlight style, must be supported by  https://github.com/alecthomas/chroma
highlight_style: monokai

# plain html to add to html <head> _before_ all Slate stylesheets and javascripts references
html_premble: |
    <link rel="stylesheet" href="https://cdn.rawgit.com/tonsky/FiraCode/1.204/distr/fira_code.css">

# An SCSS header to adjust Slate CSS
# A list of available variables can be obtained by go-slate extract . stylesheets/_variables.scss
style: |
    %default-font {
      font-family: "Fira Code", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol";
      font-size: 14px;
    }
    
```

## What go-slate is not

`go-slate` solves a very basic task and tries to be as simple and unobtrusive as possible. It is not, by any 
mean, a generic site generator. Use [Hugo](https://gohugo.io/) for that.

## License

The MIT License, see [LICENSE.md](LICENSE.md).

The embedded [Slate](https://github.com/lord/slate) is licensed under Apache 2.0 License, which can be
extracted with `go-slate extract . LICENSE` command.

# Special thanks

- [Slate](https://github.com/lord/slate) for the great API documentation layout
- [Hugo DocuAPI theme](https://github.com/bep/docuapi) for inspiration
- [Blackfriday](https://github.com/russross/blackfriday) Mardown engine for Go
- [Chroma](https://github.com/alecthomas/chroma) the syntax highlighter for Go
- [go-libsass](https://github.com/wellington/go-libsass) the [LibSass](http://sass-lang.com/libsass) Go bindings
- [Steve Francia](https://github.com/spf13) for cobra, pflags and afero
