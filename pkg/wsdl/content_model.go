package wsdl

const maxContentModelDepth = 12

// CollectElements returns the element declarations that make up a complex
// type's content model.
//
// XSD lets a content model nest compositors freely (sequence in choice, choice
// in sequence, and so on) and inherit from a base type via complexContent.
// Walking only the top-level compositor silently drops operation arguments,
// which for a scanner means attack surface that is never tested.
func CollectElements(ct *XSDComplexType, registry *TypeRegistry) []XSDElement {
	if ct == nil {
		return nil
	}
	return collectComplexTypeElements(ct, registry, make(map[string]bool), 0)
}

func collectComplexTypeElements(ct *XSDComplexType, registry *TypeRegistry, visited map[string]bool, depth int) []XSDElement {
	if ct == nil || depth > maxContentModelDepth {
		return nil
	}

	var elements []XSDElement

	if ct.ComplexContent != nil {
		if ext := ct.ComplexContent.Extension; ext != nil {
			elements = append(elements, collectBaseElements(ext.Base, registry, visited, depth)...)
			elements = append(elements, collectSequence(ext.Sequence, registry, visited, depth)...)
			elements = append(elements, collectAll(ext.All, depth)...)
			elements = append(elements, collectChoice(ext.Choice, registry, visited, depth)...)
		}
		if rest := ct.ComplexContent.Restriction; rest != nil {
			elements = append(elements, collectSequence(rest.Sequence, registry, visited, depth)...)
			elements = append(elements, collectAll(rest.All, depth)...)
			elements = append(elements, collectChoice(rest.Choice, registry, visited, depth)...)
		}
	}

	elements = append(elements, collectSequence(ct.Sequence, registry, visited, depth)...)
	elements = append(elements, collectAll(ct.All, depth)...)
	elements = append(elements, collectChoice(ct.Choice, registry, visited, depth)...)

	return elements
}

// collectBaseElements resolves a complexContent base type, guarding against the
// cyclic hierarchies that malformed or hostile schemas can declare.
func collectBaseElements(base string, registry *TypeRegistry, visited map[string]bool, depth int) []XSDElement {
	if base == "" || registry == nil {
		return nil
	}

	localName := ExtractLocalName(base)
	if IsXSDBuiltinType(localName) || visited[localName] {
		return nil
	}
	visited[localName] = true
	defer delete(visited, localName)

	baseCT, ok := registry.ComplexTypes[localName]
	if !ok {
		if baseCT, ok = registry.ComplexTypes[base]; !ok {
			return nil
		}
	}

	return collectComplexTypeElements(baseCT, registry, visited, depth+1)
}

func collectSequence(seq *XSDSequence, registry *TypeRegistry, visited map[string]bool, depth int) []XSDElement {
	if seq == nil || depth > maxContentModelDepth {
		return nil
	}

	elements := append([]XSDElement{}, seq.Elements...)
	for i := range seq.Sequences {
		elements = append(elements, collectSequence(&seq.Sequences[i], registry, visited, depth+1)...)
	}
	for i := range seq.Choices {
		elements = append(elements, collectChoice(&seq.Choices[i], registry, visited, depth+1)...)
	}
	return elements
}

func collectAll(all *XSDAll, depth int) []XSDElement {
	if all == nil || depth > maxContentModelDepth {
		return nil
	}
	return append([]XSDElement{}, all.Elements...)
}

// collectChoice returns every branch of a choice. Only one branch is valid in a
// single message, but the scanner wants each branch as a candidate input.
func collectChoice(choice *XSDChoice, registry *TypeRegistry, visited map[string]bool, depth int) []XSDElement {
	if choice == nil || depth > maxContentModelDepth {
		return nil
	}

	elements := append([]XSDElement{}, choice.Elements...)
	for i := range choice.Sequences {
		elements = append(elements, collectSequence(&choice.Sequences[i], registry, visited, depth+1)...)
	}
	for i := range choice.Choices {
		elements = append(elements, collectChoice(&choice.Choices[i], registry, visited, depth+1)...)
	}
	return elements
}
