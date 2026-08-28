package clientrecipe

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPreviewBehaviorKeepsDisclosureExplicit(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	h1 := goldenH1(t)
	h2, err := BuildH2(t.Context(), h1, &recordingH2Provider{})
	if err != nil {
		t.Fatal(err)
	}
	model, err := BuildPreviewModel(h1, h2, goldenEvaluation(t))
	if err != nil {
		t.Fatal(err)
	}
	modelRaw, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	modelPath := filepath.Join(temp, "model.json")
	scriptPath := filepath.Join(temp, "preview.js")
	runnerPath := filepath.Join(temp, "behavior.js")
	for filename, content := range map[string][]byte{
		modelPath: modelRaw, scriptPath: []byte(previewScript), runnerPath: []byte(previewBehaviorRunner),
	} {
		if err := os.WriteFile(filename, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	output, err := exec.Command(node, runnerPath, modelPath, scriptPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run client recipe preview behavior: %v\n%s", err, output)
	}
}

const previewBehaviorRunner = `
const fs = require('fs');
const vm = require('vm');

const failures = [];
function check(value, message) { if (!value) failures.push(message); }

class TestNode {
  constructor(tagName, text) {
    this.tagName = String(tagName).toUpperCase();
    this.children = [];
    this.parentNode = null;
    this.attributes = Object.create(null);
    this.listeners = Object.create(null);
    this.className = '';
    this.href = '';
    this.id = '';
    this.style = {};
    this.disabled = false;
    this._text = text || '';
    this.focused = false;
  }
  appendChild(child) { child.parentNode = this; this.children.push(child); return child; }
  replaceChildren(...children) { this.children.forEach((child) => child.parentNode = null); this.children = []; children.forEach((child) => this.appendChild(child)); }
  remove() { if (this.parentNode) this.parentNode.replaceChildren(...this.parentNode.children.filter((child) => child !== this)); }
  get firstChild() { return this.children[0] || null; }
  set textContent(value) { this._text = String(value); this.children = []; }
  get textContent() { return this._text + this.children.map((child) => child.textContent).join(''); }
  setAttribute(name, value) { this.attributes[name] = String(value); if (name === 'class') this.className = String(value); }
  getAttribute(name) { return Object.prototype.hasOwnProperty.call(this.attributes, name) ? this.attributes[name] : null; }
  addEventListener(type, listener) { (this.listeners[type] ||= []).push(listener); }
  dispatch(type, extra) {
    const event = Object.assign({ type, target: this, currentTarget: this, key: '', defaultPrevented: false,
      preventDefault() { this.defaultPrevented = true; } }, extra || {});
    (this.listeners[type] || []).forEach((listener) => listener.call(this, event));
    return event;
  }
  click() {
    if (this.disabled) return;
    const event = this.dispatch('click');
    if (!event.defaultPrevented && this.tagName === 'A' && this.href.startsWith('#')) location.hash = this.href;
  }
  focus() { this.focused = true; }
  querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
  querySelectorAll(selector) { return descendants(this).filter((node) => node !== this && matches(node, selector)); }
}

function descendants(root) {
  const rows = [];
  (function visit(node) { rows.push(node); node.children.forEach(visit); }(root));
  return rows;
}

function matches(node, selector) {
  if (selector.startsWith('.')) return node.className.split(/\s+/).includes(selector.slice(1));
  const attribute = selector.match(/^\[([^=\]]+)(?:="([^"]*)")?\]$/);
  if (attribute) {
    const value = node.getAttribute(attribute[1]);
    return value !== null && (attribute[2] === undefined || value === attribute[2]);
  }
  return node.tagName === selector.toUpperCase();
}

const app = new TestNode('main');
const overlay = new TestNode('div');
const announcer = new TestNode('div');
const targetName = new TestNode('span');
const documentListeners = Object.create(null);
const document = {
  createElement(tag) { return new TestNode(tag); },
  createTextNode(text) { return new TestNode('#text', String(text)); },
  getElementById(id) { return ({ app, 'overlay-root': overlay, 'route-announcer': announcer, 'target-chip-name': targetName })[id] || null; },
  addEventListener(type, listener) { (documentListeners[type] ||= []).push(listener); },
  dispatch(type, extra) {
    const event = Object.assign({ type, key: '', defaultPrevented: false, preventDefault() { this.defaultPrevented = true; } }, extra || {});
    (documentListeners[type] || []).forEach((listener) => listener(event));
    return event;
  }
};
const windowListeners = Object.create(null);
const location = { _hash: '#/target' };
Object.defineProperty(location, 'hash', {
  get() { return this._hash; },
  set(value) { this._hash = String(value); (windowListeners.hashchange || []).forEach((listener) => listener()); }
});
const window = {
  __CLIENT_RECIPE_MODEL__: JSON.parse(fs.readFileSync(process.argv[2], 'utf8')),
  location,
  setTimeout(callback) { callback(); },
  scrollTo() {},
  addEventListener(type, listener) { (windowListeners[type] ||= []).push(listener); }
};
const context = { console, document, window, location };
context.globalThis = context;
vm.runInNewContext(fs.readFileSync(process.argv[3], 'utf8'), context, { filename: process.argv[3] });

function state() { return app.firstChild && app.firstChild.getAttribute('data-state'); }
function action(name, root = app) { return root.querySelector('[data-action="' + name + '"]'); }
function all(selector, root = app) { return root.querySelectorAll(selector); }
function linkTo(href, root = app) { return descendants(root).find((node) => node.href === href); }

check(state() === 'target_landing', 'initial state is not target_landing');
check(all('[data-object="task_card"]').length === 6, 'landing does not expose exactly six task cards');
check(all('.locator').length === 0, 'landing materialized source locators');
check(all('[data-audit-row]').length === 0 && all('[data-audit-row]', overlay).length === 0, 'landing materialized audit rows');

action('open_recipe').click();
check(state() === 'recipe_overview', 'open recipe did not enter overview');
check(all('[data-action="open_step"]').length === 6, 'overview does not expose six recipe steps');
check(all('.example-card').length === 3, 'overview did not start with exactly three examples');
check(!app.textContent.includes('Notifier'), 'incomplete fourth example was materialized initially');
check(all('.locator').length === 0, 'overview materialized source locators');
check(all('[data-audit-row]', overlay).length === 0, 'overview materialized audit rows before disclosure');

const showAll = action('show_all_examples');
showAll.click();
check(showAll.getAttribute('aria-expanded') === 'true', 'show-all control did not expose aria-expanded');
check(all('.example-card').length === 4 && app.textContent.includes('Notifier'), 'show-all did not reveal the incomplete example');
check(app.textContent.includes('Verification, Observability, Failure policy'), 'Notifier did not expose its exact three missing roles');

window.location.hash = '#/recipe/step/s1';
check(state() === 'recipe_step', 'step hash did not enter recipe_step');
check(all('.locator').length === 0, 'step materialized evidence before the action');
action('open_evidence').click();
check(state() === 'evidence', 'step evidence action did not enter evidence state');
check(all('.evidence-card').length === 3, 'evidence state did not honor the initial three-locator bound');
const source = action('open_exact_source');
check(source && source.href === '../repo/internal/clients/kubernetes/config.go#L10', 'first exact source action is not the validated Kubernetes locator');
action('return_detail').click();
check(state() === 'recipe_step', 'evidence back action did not restore the step');

window.location.hash = '#/recipe';
action('show_all_examples').click();
const notifierLink = linkTo('#/recipe/example/e3');
check(Boolean(notifierLink), 'all-examples view has no Notifier link');
notifierLink.click();
check(state() === 'example_instance', 'Notifier link did not enter example state');
check(app.textContent.includes('Needs 3 roles'), 'Notifier detail lost incomplete status');
check(all('[data-object="slot_map"]').length === 1, 'example detail has no grouped slot map');
check(all('.slot-card').length === 6, 'example detail does not expose six grouped slots');
check(all('.slot-card').filter((node) => node.className.split(/\s+/).includes('missing')).length === 2, 'Notifier grouped slots do not preserve its missing roles');
action('open_evidence').click();
check(state() === 'evidence', 'example evidence action did not enter evidence state');
check(all('.evidence-card').length === 1, 'example slot did not disclose its exact evidence only');
check(action('open_exact_source').href === '../repo/internal/clients/notifier/config.go#L9', 'example evidence lost its exact source href');
action('return_detail').click();
check(state() === 'example_instance', 'example evidence back action did not restore the example');

window.location.hash = '#/recipe';
const auditButton = action('open_audit');
auditButton.click();
check(auditButton.getAttribute('aria-expanded') === 'true', 'audit control did not expose aria-expanded');
check(all('[data-audit-row]', overlay).length === 6, 'audit drawer did not create exactly six exclusion rows');
document.dispatch('keydown', { key: 'Escape' });
check(!overlay.firstChild && auditButton.getAttribute('aria-expanded') === 'false', 'Escape did not close the audit disclosure');

if (failures.length) {
  console.error(failures.join('\n'));
  process.exit(1);
}
console.log('client recipe preview behavior: PASS');
`
