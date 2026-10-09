package clojureproject

import "testing"

func TestDocumentationUsesNativeUTF16FormAndLiteralColumns(t *testing.T) {
	docs := Docstrings([]byte("(ns example.same-line)\n(defn first \"Описание 😀.\" [] 1) (defn second \"Second owns its words.\" [] 2)\n"))
	// The actual clj-kondo var-definition source positions are (2,1)/(2,34).
	if len(docs) != 2 || docs[0].DeclarationLine != 2 || docs[0].DeclarationColumn != 1 || docs[1].DeclarationLine != 2 || docs[1].DeclarationColumn != 34 || docs[1].Column != 47 {
		t.Fatalf("native form/literal positions: %+v", docs)
	}
}
