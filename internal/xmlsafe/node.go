package xmlsafe

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

type Node struct {
	Name     string
	Space    string
	Text     string
	Attrs    map[string]string
	Children []*Node
}

func Parse(data []byte) (*Node, error) {
	if len(data) > 4<<20 {
		return nil, errors.New("XML response exceeds 4 MiB")
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	var root *Node
	stack := []*Node{}
	count := 0
	for {
		tok, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, errors.New("malformed device XML")
		}
		switch x := tok.(type) {
		case xml.StartElement:
			count++
			if count > 50000 || len(stack) > 32 {
				return nil, errors.New("device XML nesting or node limit exceeded")
			}
			n := &Node{Name: x.Name.Local, Space: x.Name.Space, Attrs: map[string]string{}}
			for _, a := range x.Attr {
				if len(a.Value) > 4096 {
					return nil, errors.New("oversized XML attribute")
				}
				n.Attrs[a.Name.Local] = a.Value
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("multiple XML roots")
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				p.Children = append(p.Children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, errors.New("unbalanced XML")
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				n := stack[len(stack)-1]
				if len(n.Text)+len(x) > 16384 {
					return nil, errors.New("oversized XML text")
				}
				n.Text += string(x)
			} else if strings.TrimSpace(string(x)) != "" {
				return nil, errors.New("unexpected XML text")
			}
		case xml.Directive:
			return nil, errors.New("XML directives are not supported")
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, errors.New("incomplete device XML")
	}
	return root, nil
}
func (n *Node) At(path ...string) *Node {
	for _, p := range path {
		if n == nil {
			return nil
		}
		var next *Node
		for _, c := range n.Children {
			if c.Name == p {
				next = c
				break
			}
		}
		n = next
	}
	return n
}
func (n *Node) Value(path ...string) string {
	x := n.At(path...)
	if x == nil {
		return ""
	}
	return strings.TrimSpace(x.Text)
}
func (n *Node) All(name string) []*Node {
	out := []*Node{}
	if n == nil {
		return out
	}
	if n.Name == name {
		out = append(out, n)
	}
	for _, c := range n.Children {
		out = append(out, c.All(name)...)
	}
	return out
}
func (n *Node) Attribute(k string) string {
	if n == nil {
		return ""
	}
	return n.Attrs[k]
}
