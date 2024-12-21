package govalidtemple

import (
	"errors"
	"fmt"
	"html/template"
	"reflect"
	"text/template/parse"
)

func ValidateViewModel(data interface{}, tmpl *template.Template, templateName string) error {
	missing, extra := CompareViewModel(data, tmpl, templateName)
	errorStr := ""
	if len(extra) > 0 {
		errorStr += "extra fields ["
		for idx, item := range extra {
			if idx > 0 {
				errorStr += ", "
			}
			errorStr += item
		}
		errorStr += "] "
	}
	if len(missing) > 0 {
		errorStr += "missing fields ["
		for idx, item := range missing {
			if idx > 0 {
				errorStr += ", "
			}
			errorStr += item
		}
		errorStr += "]"
	}
	if len(extra) > 0 || len(missing) > 0 {
		return errors.New(errorStr)
	}

	return nil
}

type templateField struct {
	Name     string                    // Name of the field
	Children map[string]*templateField // Nested fields (e.g., for structs or maps)
}

func newTemplateField(name string) *templateField {
	return &templateField{
		Name:     name,
		Children: make(map[string]*templateField),
	}
}

func (tf *templateField) addChild(name string) *templateField {
	child, exists := tf.Children[name]
	if !exists {
		child = newTemplateField(name)
		tf.Children[name] = child
	}
	return child
}

func extractFieldsFromTemplate(root *template.Template, n parse.Node, parentField *templateField) {
	switch node := n.(type) {
	case *parse.PipeNode:
		for _, cmd := range node.Cmds {
			for _, arg := range cmd.Args {
				if field, ok := arg.(*parse.FieldNode); ok {
					// Add the field to the parent field's children
					fieldParts := field.Ident
					current := parentField
					for _, part := range fieldParts {
						current = current.addChild(part)
					}
				}
			}
		}
	case *parse.ActionNode:
		for _, cmd := range node.Pipe.Cmds {
			for _, arg := range cmd.Args {
				if field, ok := arg.(*parse.FieldNode); ok {
					// Add the field to the parent field's children
					fieldParts := field.Ident
					current := parentField
					for _, part := range fieldParts {
						current = current.addChild(part)
					}
				}
			}
		}
	case *parse.TemplateNode:
		// Handle included templates ({{ template "name" .Field }})
		if node.Pipe != nil {
			for _, cmd := range node.Pipe.Cmds {
				for _, arg := range cmd.Args {
					if field, ok := arg.(*parse.FieldNode); ok {
						// Locate the parent for the included template's fields
						fieldParts := field.Ident
						current := parentField
						for _, part := range fieldParts {
							current = current.addChild(part)
						}
						// Recursively extract fields from the included template under this parent
						if tmpl := root.Lookup(node.Name); tmpl != nil && tmpl.Tree != nil {
							extractFieldsFromTemplate(root, tmpl.Tree.Root, current)
						}
					} else if _, ok := arg.(*parse.DotNode); ok {
						// Recursively extract fields from the included template under this parent
						if tmpl := root.Lookup(node.Name); tmpl != nil && tmpl.Tree != nil {
							extractFieldsFromTemplate(root, tmpl.Tree.Root, parentField)
						}
					}
				}
			}
		} else {
			// If no specific argument is passed, inherit from the parent
			if tmpl := root.Lookup(node.Name); tmpl != nil && tmpl.Tree != nil {
				extractFieldsFromTemplate(root, tmpl.Tree.Root, parentField)
			}
		}
	case *parse.ListNode:
		for _, child := range node.Nodes {
			extractFieldsFromTemplate(root, child, parentField)
		}
	case *parse.IfNode:
		extractFieldsFromTemplate(root, node.List, parentField)
		if node.ElseList != nil {
			extractFieldsFromTemplate(root, node.ElseList, parentField)
		}
	case *parse.RangeNode:
		extractFieldsFromTemplate(root, node.Pipe, parentField)
	case *parse.WithNode:
		extractFieldsFromTemplate(root, node.List, parentField)
	}
}

func extractFieldsFromStruct(v interface{}) *templateField {
	root := newTemplateField("Root")
	extractHelper(reflect.TypeOf(v), root)
	return root
}

func extractHelper(typ reflect.Type, parentField *templateField) {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" { // Skip unexported fields
			continue
		}

		child := parentField.addChild(field.Name)
		if field.Type.Kind() == reflect.Struct {
			extractHelper(field.Type, child)
		}
	}
}

func CompareViewModel(data interface{}, tmpl *template.Template, templateName string) (missing []string, extra []string) {
	// Extract fields from the template
	rootTemplateField := newTemplateField("Root")
	tmplTree := tmpl.Lookup(templateName)
	if tmplTree == nil || tmplTree.Tree == nil {
		panic(fmt.Sprintf("template %q not found", templateName))
	}
	extractFieldsFromTemplate(tmpl, tmplTree.Tree.Root, rootTemplateField)

	// Extract fields from the struct
	rootStructField := extractFieldsFromStruct(data)

	// Compare the two field sets
	return compareTemplateFields(rootTemplateField, rootStructField)
}

func compareTemplateFields(templateField, structField *templateField) (missing []string, extra []string) {
	for name, child := range templateField.Children {
		if _, ok := structField.Children[name]; !ok {
			missing = append(missing, structField.Name+"->"+name)
		} else {
			m, e := compareTemplateFields(child, structField.Children[name])
			missing = append(missing, m...)
			extra = append(extra, e...)
		}
	}
	for name := range structField.Children {
		if _, ok := templateField.Children[name]; !ok {
			extra = append(extra, templateField.Name+"->"+name)
		}
	}
	return missing, extra
}
