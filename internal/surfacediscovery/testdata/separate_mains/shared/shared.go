package shared

var callback func()

func Set(next func()) {
	callback = next
}

func Run() {
	callback()
}
