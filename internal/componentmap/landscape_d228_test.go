package componentmap

import "testing"

// Decision 228: etcd-class proposal — 16 honest components in ONE subsystem
// — must be accepted (was rejected by MaxComponentsPerSubsystem=8).
func TestApplyAcceptsEtcdClassManyComponentsInOneSubsystem(t *testing.T) {
	t.Parallel()

	bundle := candidateBundleWithPackages(16)
	ids := candidateIDs(bundle.Candidates)
	components := make([]ProposedComponent, 16)
	for index := 0; index < 16; index++ {
		components[index] = ProposedComponent{
			Name:      []string{"Ядро сервера", "Хранилище", "Сервисный слой", "Сетевой транспорт", "Встраиваемый сервер", "Клиентская библиотека", "Интеграционное тестирование", "Утилиты и вспомогательные пакеты", "Прокси и шлюзы", "Экспериментальные функции", "Моки и заглушки", "Внутренние пакеты", "Тестовые фреймворки", "Устойчивость и отказоустойчивость", "Вспомогательные инструменты", "Локальный остаток"}[index],
			MemberIDs: []MemberID{ids[index]},
		}
	}
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Etcd-like", Components: components,
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Fallback || result.ValidationOutcome == ValidationRejected {
		t.Fatalf("etcd-class 16-component subsystem was rejected: %#v", result.Diagnostics)
	}
	if len(result.Subsystems[0].Components) != 16 {
		t.Fatalf("components = %d, want 16", len(result.Subsystems[0].Components))
	}
}

// Decision 228: hypothesis is advisory — a model flag that differs from the
// backend derivation must not reject the proposal; the backend derives and
// overwrites it.
func TestApplyAcceptsAdvisoryHypothesisMismatch(t *testing.T) {
	t.Parallel()

	bundle := candidateBundleWithPackages(2)
	ids := candidateIDs(bundle.Candidates)
	// Model claims grounded (hypothesis=false) but the component lacks any
	// operational anchor → backend derives hypothesis=true.
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Repository",
			Components: []ProposedComponent{
				{Name: "Package area", MemberIDs: ids, Hypothesis: false},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Fallback || result.ValidationOutcome == ValidationRejected {
		t.Fatalf("advisory hypothesis mismatch was rejected: %#v", result.Diagnostics)
	}
	component := result.Subsystems[0].Components[0]
	if !component.Hypothesis {
		t.Fatalf("backend did not derive hypothesis=true for an ungrounded component: %#v", component)
	}
}
