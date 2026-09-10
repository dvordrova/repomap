package main

// These are source observations, without a database connection or execution.
var sqlSourceExamples = []string{
	"SELECT " + "id FROM public.orders WHERE note = 'JOIN imaginary_table'",
	"SELECT 1",
	"SELECT 'ready'",
	"WITH recent AS (SELECT id FROM public.orders) SELECT id FROM recent",
	"UPDATE public.orders SET note = 'updated'",
	"SELECT * FROM \"unterminated",
	"SELECT {projection}",
	"UPDATE public.orders AS o SET id=2",
	"SELECT CASE status WHEN 1 THEN 'open' ELSE 'closed' END FROM public.orders",
	"SELECT note COLLATE \"C\" FROM public.orders",
	"SELECT CURRENT_USER UNION SELECT note FROM public.orders",
}

var ordinarySourceText = []string{
	"create-userdir",
	"Create a new strategy from a template",
	"Create user-data directory.",
	"Select Trading mode",
	"Insert Exchange API Key",
	"Update trades from arguments",
	"Delete files from cache",
	"With values from Arguments",
	"DELETE",
	"SELECT id",
	"SELECT + \"\"",
}
