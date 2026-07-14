package componentprobe

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

func stableID(prefix string, parts ...string) string {
	hash := sha256.New()
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(part))
	}
	return fmt.Sprintf("%s-%x", prefix, hash.Sum(nil)[:10])
}

func buildEvidenceIndex(probe SymbolProbe) []EvidenceRef {
	refs := make([]EvidenceRef, 0,
		1+len(probe.Structural.Candidates)+
			len(probe.Structural.IncomingCalls)+len(probe.Structural.OutgoingCalls)+
			len(probe.Source.Lines)+len(probe.Tests.References),
	)
	refs = appendEvidenceRef(refs, probe.ID, ArtifactStructural, EvidenceResolution, probe.Structural.Target.EvidenceID, SupportStaticNavigation)
	for _, fact := range probe.Structural.Candidates {
		refs = appendEvidenceRef(refs, probe.ID, ArtifactStructural, EvidenceCandidate, fact.EvidenceID, SupportStaticNavigation)
	}
	for _, call := range probe.Structural.IncomingCalls {
		refs = appendEvidenceRef(refs, probe.ID, ArtifactStructural, EvidenceIncomingCall, call.EvidenceID, SupportStaticNavigation)
	}
	for _, call := range probe.Structural.OutgoingCalls {
		refs = appendEvidenceRef(refs, probe.ID, ArtifactStructural, EvidenceOutgoingCall, call.EvidenceID, SupportStaticNavigation)
	}
	for _, line := range probe.Source.Lines {
		refs = appendEvidenceRef(refs, probe.ID, ArtifactSource, EvidenceSourceLine, line.EvidenceID, SupportSource)
	}
	for _, reference := range probe.Tests.References {
		refs = appendEvidenceRef(refs, probe.ID, ArtifactTests, EvidenceTestReference, reference.EvidenceID, SupportTestNavigation)
	}
	return refs
}

func appendEvidenceRef(
	refs []EvidenceRef,
	probeID string,
	artifact ArtifactKind,
	kind EvidenceKind,
	localID string,
	basis SupportBasis,
) []EvidenceRef {
	origin := EvidenceOrigin{ProbeID: probeID, Artifact: artifact, LocalID: localID}
	return append(refs, EvidenceRef{
		ID:      stableID("ev", probeID, string(artifact), localID),
		Kind:    kind,
		LocalID: localID,
		Origin:  origin,
		Basis:   basis,
	})
}
