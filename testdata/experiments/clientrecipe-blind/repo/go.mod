module example.invalid/fulfilment

go 1.22

require (
	acquirer.example/authorizer v0.0.0
	geo.example/locator v0.0.0
	messaging.example/receipt v0.0.0
	revenue.example/taxquote v0.0.0
	shipping.example/parcel v0.0.0
)

replace acquirer.example/authorizer => ../deps/authorizer

replace geo.example/locator => ../deps/locator

replace messaging.example/receipt => ../deps/receipt

replace revenue.example/taxquote => ../deps/taxquote

replace shipping.example/parcel => ../deps/parcel
