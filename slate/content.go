package slate

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/alecthomas/chroma"
	chroma_html "github.com/alecthomas/chroma/formatters/html"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/styles"
	"github.com/russross/blackfriday"
	"gopkg.in/yaml.v2"
	"html"
	"io/ioutil"
	"path/filepath"
	"sort"
	"text/template"
	"reflect"
	"encoding/json"
	"github.com/spf13/afero"
	"github.com/tdewolff/minify"
	minify_html "github.com/tdewolff/minify/html"
	"github.com/growler/go-slate/slate/internal/slate"
	"path"
)

type ContentParams struct {
	Title      string   `yaml:"title,omitempty"`
	Search     bool     `yaml:"search,omitempty"`
	Highlight  string   `yaml:"highlight_style,omitempty"`
	Langs      []string `yaml:"language_tabs,omitempty"`
	TocFooters []string `yaml:"toc_footers,omitempty"`
	Includes   []string `yaml:"includes,omitempty"`
	Style      string   `yaml:"style,omitempty"`
	Logo       string   `yaml:"logo,omitempty"`
	RTLEnabled bool     `yaml:"enable_rtl,omitempty"`
	HTMLHead   string   `yaml:"html_head,omitempty"`
}

type chromaTypes struct {
	types []chroma.TokenType
	names []string
}

func (ct *chromaTypes) Len() int {
	return len(ct.types)
}

func (ct *chromaTypes) Less(i, j int) bool {
	return ct.types[i] < ct.types[j]
}

func (ct *chromaTypes) Swap(i, j int) {
	t := ct.types[i]
	n := ct.names[i]
	ct.types[i] = ct.types[j]
	ct.names[i] = ct.names[j]
	ct.types[j] = t
	ct.names[j] = n
}

var ct chromaTypes

func init() {
	ct.types = make([]chroma.TokenType, 0, len(chroma.StandardTypes))
	ct.names = make([]string, 0, len(chroma.StandardTypes))
	for typ, name := range chroma.StandardTypes {
		ct.types = append(ct.types, typ)
		ct.names = append(ct.names, name)
	}
	sort.Sort(&ct)
}

func (p *ContentParams) StyleCSS() string {
	var style *chroma.Style
	if p.Highlight == "" {
		style = styles.Monokai
	} else {
		style = styles.Get(p.Highlight)
	}
	var buf bytes.Buffer
	var bg = style.Get(chroma.Background)
	fmt.Fprintf(&buf, "\n.highlight pre { %s }", chroma_html.StyleEntryToCSS(bg))
	fmt.Fprintf(&buf, "\n.highlight .hll { %s }", chroma_html.StyleEntryToCSS(bg))
	for i, typ := range ct.types {
		entry := style.Get(typ)
		if typ != chroma.Background {
			entry = entry.Sub(bg)
		}
		if entry.IsZero() {
			continue
		}
		fmt.Fprintf(&buf, "\n.highlight .%s { %s }", ct.names[i], chroma_html.StyleEntryToCSS(entry))
	}
	return buf.String()
}

type content struct {
	html   []byte
	Params ContentParams
}

func load(fs slate.FileSystem, params Params) (*content, error) {
	tmplFile, err := fs.Open("layouts/layout.tmpl")
	if err != nil {
		return nil, err
	}
	tmplSrc, err := ioutil.ReadAll(tmplFile)
	tmplFile.Close()
	if err != nil {
		return nil, err
	}
	tmpl := template.New("layout")
	tmpl.Funcs(template.FuncMap{
		"json": func(arg0 reflect.Value) (reflect.Value, error) {
			if data, err := json.Marshal(arg0.Interface()); err != nil {
				return reflect.Value{}, err
			} else {
				return reflect.ValueOf(string(data)), nil
			}
		},
	})
	if tmpl, err = tmpl.Parse(string(tmplSrc)); err != nil {
		return nil, err
	}
	file, err := fs.Open("index.html.md")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	buf := bytes.Buffer{}
	preamble := bytes.Buffer{}
	lineReader := bufio.NewScanner(file)
	state := 0
	for lineReader.Scan() {
		line := lineReader.Text()
		switch state {
		case 0:
			if line == "---" {
				state = 1
				continue
			} else {
				state = 2
			}
		case 1:
			if line == "---" {
				state = 2
				continue
			} else {
				preamble.WriteString(line)
				preamble.WriteByte('\n')
				continue
			}
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	ret := &content{}
	if err = yaml.Unmarshal(preamble.Bytes(), &ret.Params); err != nil {
		return nil, err
	}
	for _, include := range ret.Params.Includes {
		inc, err := fs.Open(path.Join("includes", "_" + include + ".md"))
		if err != nil {
			return nil, err
		}
		data, err := ioutil.ReadAll(inc)
		inc.Close()
		if err != nil {
			return nil, err
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	if params.LogoFile != "" {
		ret.Params.Logo = params.LogoFile
	}
	if params.Search != nil {
		ret.Params.Search = *params.Search
	}
	if params.RTL != nil {
		ret.Params.RTLEnabled = *params.RTL
	}
	parser := blackfriday.New(blackfriday.WithExtensions(
		blackfriday.CommonExtensions | blackfriday.AutoHeadingIDs,
	))
	htmlRenderer := blackfriday.NewHTMLRenderer(blackfriday.HTMLRendererParameters{})
	ast := parser.Parse(buf.Bytes())
	toc := produceTOC(htmlRenderer, ast)
	con := produceHTML(htmlRenderer, ast)
	buf.Reset()
	err = tmpl.Execute(&buf, map[string]interface{}{
		"Params": &ret.Params,
		"TOC": string(toc),
		"Content": string(con),
	})
	ret.html = buf.Bytes()
	return ret, nil
}

func produceTOC(r blackfriday.Renderer, ast *blackfriday.Node) []byte {
	buf := bytes.Buffer{}

	headings := make([]*blackfriday.Node, 0, 8)

	var currentHeading *blackfriday.Node
	var currentText bytes.Buffer

	ast.Walk(func(node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
		if node.Type == blackfriday.Heading && !node.HeadingData.IsTitleblock && node.Level < 3 {
			if entering {
				currentHeading = node
				headings = append(headings, node)
			} else {
				currentHeading.Title = make([]byte, currentText.Len())
				copy(currentHeading.Title, currentText.Bytes())
				currentText.Reset()
				currentHeading = nil
			}
		} else if currentHeading != nil {
			r.RenderNode(&currentText, node, entering)
		}
		return blackfriday.GoToNext
	})
	currentLevel := 0
	for _, h := range headings {
		if currentLevel == h.Level {
			fmt.Fprintf(&buf, "</li>\n")
		} else if currentLevel != 0 && currentLevel < h.Level {
			for i := currentLevel; i < h.Level; i++ {
				fmt.Fprintf(&buf, "<ul class=\"toc-list-h%d\">\n", i+1)
			}
		} else if currentLevel > h.Level {
			for i := h.Level; i < currentLevel; i++ {
				fmt.Fprint(&buf, "</li>\n</ul>\n")
			}
		}
		currentLevel = h.Level
		title := html.EscapeString(string(h.Title))
		fmt.Fprintf(&buf, "<li>\n<a href=\"#%s\" class=\"toc-h%d toc-link\" data-title=\"%s\">%s</a>\n", h.HeadingID, h.Level, title, title)
	}
	for i := 1; i < currentLevel; i++ {
		fmt.Fprint(&buf, "</li>\n</ul>\n")
	}
	if currentLevel > 0 {
		fmt.Fprint(&buf, "</li>\n")
	}
	return buf.Bytes()
}

func produceHTML(r blackfriday.Renderer, ast *blackfriday.Node) []byte {
	var buf bytes.Buffer
	ast.Walk(func(node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
		switch node.Type {
		case blackfriday.CodeBlock:
			lang := string(node.Info)
			code := string(node.Literal)
			fmt.Fprintf(&buf, "\n<pre class=\"highlight %s tab-%s\"><code>", lang, lang)
			lexer := lexers.Get(lang)
			if lexer == nil {
				lexer = lexers.Fallback
			}
			tokens, err := lexer.Tokenise(nil, code)
			if err == nil {
				for _, tok := range tokens.Tokens() {
					if name, ok := chroma.StandardTypes[tok.Type]; ok && name != "" {
						fmt.Fprintf(&buf, "<span class=\"%s\">%s</span>", name, html.EscapeString(tok.Value))
					} else {
						fmt.Fprint(&buf, html.EscapeString(tok.Value))
					}
				}
			} else {
				fmt.Fprintln(&buf, html.EscapeString(code))
			}
			fmt.Fprint(&buf, "</code></pre>\n")
			return blackfriday.GoToNext
		default:
			return r.RenderNode(&buf, node, entering)
		}
	})
	return buf.Bytes()
}

func (c *content) produce(target *afero.Afero, minifyHTML bool) error {
	out, err := target.TempFile(".", ".slate")
	if err != nil {
		return err
	}
	outName := filepath.Base(out.Name())
	defer func() {
		out.Close()
		target.Remove(outName)
	}()
	var result []byte
	if minifyHTML {
		m := minify.New()
		if minifyHTML {
			m.AddFunc("text/html", minify_html.Minify)
		}
		result, err = m.Bytes("text/html", c.html)
		if err != nil {
			return err
		}
	} else {
		result = c.html
	}
	if _, err = out.Write(result); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	if err = target.Rename(outName, "index.html"); err != nil {
		return err
	}
	return nil
}