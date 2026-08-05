package web

// Namespace is a beego route namespace descriptor. Route paths are bound to
// controller methods at registration time; the exact path surface is not a
// plain (path, handler) constant pair.
type Namespace struct {
	prefix string
	items  []any
}

type Interface interface{}

func NewNamespace(prefix string, params ...any) *Namespace {
	return &Namespace{prefix: prefix, items: params}
}

func NSNamespace(subprefix string, params ...any) any {
	return &Namespace{prefix: subprefix, items: params}
}

func NSInclude(controllers ...any) any {
	return controllers
}

func NSRouter(rootpath string, c Interface, mappingMethod string, methods ...string) any {
	return nil
}

func AddNamespace(ns *Namespace) {}
