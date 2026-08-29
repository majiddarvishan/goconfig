package goconfig

import "strconv"

func findNodePath(parentNode *Node, desiredNode *Node) string {
	if parentNode == desiredNode {
		return ""
	}

	var pathSegments []string
	if findNodePathRecursive(parentNode, desiredNode, &pathSegments) {
		return buildJSONPointer(pathSegments)
	}
	return ""
}

func findNodePathRecursive(parentNode *Node, desiredNode *Node, pathSegments *[]string) bool {
	if parentNode == desiredNode {
		return true
	}

	switch parentNode.Type() {
	case Array:
		array, err := parentNode.GetArray()
		if err != nil {
			return false
		}
		for index, child := range array {
			*pathSegments = append(*pathSegments, strconv.Itoa(index))
			if findNodePathRecursive(child, desiredNode, pathSegments) {
				return true
			}
			*pathSegments = (*pathSegments)[:len(*pathSegments)-1]
		}
	case Object:
		object, err := parentNode.GetObject()
		if err != nil {
			return false
		}
		for key, child := range object {
			*pathSegments = append(*pathSegments, key)
			if findNodePathRecursive(child, desiredNode, pathSegments) {
				return true
			}
			*pathSegments = (*pathSegments)[:len(*pathSegments)-1]
		}
	}
	return false
}
