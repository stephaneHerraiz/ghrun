package gh

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrNoDispatch indicates a workflow has no workflow_dispatch trigger.
var ErrNoDispatch = errors.New("workflow has no workflow_dispatch trigger")

// WorkflowInputs fetches the workflow file and parses its workflow_dispatch inputs in order.
func (c *Client) WorkflowInputs(repo RepoRef, path string) ([]Input, error) {
	out, err := c.run.Exec("api",
		fmt.Sprintf("repos/%s/%s/contents/%s", repo.Owner, repo.Name, path),
		"--jq", ".content")
	if err != nil {
		return nil, err
	}
	// Strip both \n and \r: GitHub line-wraps base64 with \n, but some proxies
	// inject \r\n, which would otherwise make the decode fail on a stray \r.
	cleaned := strings.NewReplacer("\n", "", "\r", "").Replace(strings.TrimSpace(string(out)))
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, fmt.Errorf("decoding workflow content: %w", err)
	}
	return parseDispatchInputs(decoded)
}

// parseDispatchInputs extracts on.workflow_dispatch.inputs preserving order.
func parseDispatchInputs(yml []byte) ([]Input, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(yml, &root); err != nil {
		return nil, fmt.Errorf("parsing workflow yaml: %w", err)
	}
	if len(root.Content) == 0 {
		return nil, ErrNoDispatch
	}
	doc := root.Content[0] // mapping node for the document
	onNode := mapValue(doc, "on")
	if onNode == nil {
		return nil, ErrNoDispatch
	}
	dispatch := mapValue(onNode, "workflow_dispatch")
	if dispatch == nil {
		return nil, ErrNoDispatch
	}
	inputsNode := mapValue(dispatch, "inputs")
	if inputsNode == nil || inputsNode.Kind != yaml.MappingNode {
		return []Input{}, nil // dispatch with no inputs
	}
	var inputs []Input
	// Mapping node content is [key, value, key, value, ...]; iterate in pairs to keep order.
	for i := 0; i+1 < len(inputsNode.Content); i += 2 {
		name := inputsNode.Content[i].Value
		spec := inputsNode.Content[i+1]
		inputs = append(inputs, inputFromNode(name, spec))
	}
	return inputs, nil
}

// mapValue returns the value node for key in a mapping node, or nil.
func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func inputFromNode(name string, spec *yaml.Node) Input {
	in := Input{Name: name, Type: InputString}
	if spec == nil || spec.Kind != yaml.MappingNode {
		return in
	}
	if n := mapValue(spec, "description"); n != nil {
		in.Description = n.Value
	}
	if n := mapValue(spec, "type"); n != nil {
		in.Type = InputType(n.Value)
	}
	if n := mapValue(spec, "default"); n != nil {
		in.Default = n.Value
	}
	if n := mapValue(spec, "required"); n != nil {
		in.Required = n.Value == "true"
	}
	if n := mapValue(spec, "options"); n != nil && n.Kind == yaml.SequenceNode {
		for _, o := range n.Content {
			in.Options = append(in.Options, o.Value)
		}
	}
	return in
}
