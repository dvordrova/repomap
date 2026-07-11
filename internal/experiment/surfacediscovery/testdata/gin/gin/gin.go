package gin

type Context struct{}
type HandlerFunc func(*Context)
type RouterGroup struct{}

func (group *RouterGroup) Handle(method, path string, handlers ...HandlerFunc) {}

func (group *RouterGroup) GET(path string, handler HandlerFunc) {
	group.Handle("GET", path, handler)
}
