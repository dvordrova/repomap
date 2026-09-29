# cmd/vfs builds only with the fixturevfs tag (cmd/vfs/main.go, root_vfs.go),
# as litestream's cmd/litestream-vfs does: target discovery reads this recipe.
VFS_TAGS := fixturevfs

.PHONY: vfs
vfs:
	go build -tags $(VFS_TAGS) -o bin/vfs ./cmd/vfs
