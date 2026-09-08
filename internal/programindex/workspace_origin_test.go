package programindex

import "testing"

func TestExternalWorkspaceOriginIsCanonicalAndSealed(t *testing.T) {
	input := shapeInput()
	input.Objects = append(input.Objects, ObjectInput{
		SourceRef: "workspace-get", Kind: ObjectExternalSymbol, Name: "got.get", Visibility: VisibilityPublic,
		External: &ExternalSymbol{AuthorityKind: ExternalAuthorityPackage, PackagePath: "got", Name: "get", RepositoryPath: "packages/库"},
	})
	index, err := newMeasuredProgramIndex(input)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	object := objectWithSourceRef(t, restored, "workspace-get")
	if object.External.RepositoryPath != "packages/库" {
		t.Fatalf("workspace origin lost: %#v", object)
	}
	for _, object := range index.Objects {
		if object.SourceRef == "workspace-get" {
			object.External.RepositoryPath = "packages/other"
		}
	}
	if err := index.Validate(); err == nil {
		t.Fatal("modified workspace origin escaped the seal")
	}
	for _, directory := range []string{"..", "../other", "/other", "a/../b", "a\\b", " a", "a\nb"} {
		input.Objects[len(input.Objects)-1].External.RepositoryPath = directory
		if _, err := newMeasuredProgramIndex(input); err == nil {
			t.Fatalf("invalid workspace directory accepted: %q", directory)
		}
	}
	input.Objects[len(input.Objects)-1].External.RepositoryPath = "."
	if _, err := newMeasuredProgramIndex(input); err != nil {
		t.Fatalf("root workspace package: %v", err)
	}
	input.Objects[len(input.Objects)-1].External.AuthorityKind = ExternalAuthorityPlatform
	if _, err := newMeasuredProgramIndex(input); err == nil {
		t.Fatal("platform gained workspace origin")
	}
}
