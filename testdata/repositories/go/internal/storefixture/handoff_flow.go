package storefixture

// Shared branches stay distinct paths even when they reach one callback.
type flowCallback func()
type routedCallback func()
type savedCallback func()

func flowAction() {}

func SharedCallbackFlow(flag bool, unknown flowCallback) {
	callback := flowCallback(flowAction)
	if flag {
		callback = unknown
	}
	if flag {
		callback = flowCallback(routedCallback(callback))
	} else {
		callback = flowCallback(savedCallback(callback))
	}
	if flag {
		callback = flowCallback(routedCallback(callback))
	} else {
		callback = flowCallback(savedCallback(callback))
	}
	callback()
}

func CyclicCallbackFlow(remaining int) {
	callback := flowCallback(flowAction)
	for remaining > 0 {
		callback = flowCallback(routedCallback(callback))
		remaining--
	}
	callback()
}

// Reusing a value must not merge its two exact assignment locations.
func SharedInterfaceStores(first, second *fieldFacade) {
	value := FieldStore(&storedEngine{})
	first.store = value
	second.store = value
}

func flowStoreBase() FieldStore { return &storedEngine{} }
func flowStoreBranch(flag bool) FieldStore {
	if flag {
		return flowStoreBase()
	}
	return flowStoreBase()
}
func flowStoreDiamond(flag bool) FieldStore {
	if flag {
		return flowStoreBranch(flag)
	}
	return flowStoreBranch(flag)
}
func recursiveFlowStore(flag bool) FieldStore {
	if flag {
		return &storedEngine{}
	}
	return recursiveFlowStore(flag)
}
func flowStorePair() (FieldStore, FieldStore) {
	return &storedEngine{}, &alternateEngine{}
}
func InvokeInterfaceFlows(flag bool) {
	flowStoreDiamond(flag).Put("diamond")
	recursiveFlowStore(flag).Put("cycle")
	first, second := flowStorePair()
	first.Put("first")
	second.Put("second")
}
