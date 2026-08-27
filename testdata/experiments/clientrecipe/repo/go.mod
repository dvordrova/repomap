module example.com/launchservice

go 1.23

require (
	example.com/clickhousesdk v0.0.0
	example.com/kubernetessdk v0.0.0
	example.com/legacysdk v0.0.0
	example.com/notifiersdk v0.0.0
	example.com/vaultsdk v0.0.0
)

replace example.com/clickhousesdk => ../deps/clickhousesdk

replace example.com/kubernetessdk => ../deps/kubernetessdk

replace example.com/legacysdk => ../deps/legacysdk

replace example.com/notifiersdk => ../deps/notifiersdk

replace example.com/vaultsdk => ../deps/vaultsdk
