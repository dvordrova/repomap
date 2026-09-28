(ns example.core
  (:require [example.service :as service] [clojure.string :as str] [example.facade :as facade] [clojure.java.shell :as shell]) (:import (java.util ArrayList)))

(defn -main [& names]
  (service/deliver! "greeting.txt" (service/greet (first names))))

(defn with-shadow [greet]
  ;; This must not resolve to example.service/greet.
  (greet "local"))

(defn greet-many [names]
  (map service/greet names))

(defn read-limit [] service/source-limit)

(defn shadow-limit [source-limit] source-limit)

;; This call belongs to namespace evaluation, not to a function above it.
(service/greet "loaded")

;; A statement handed to a function the caller supplies is SQL; a message that
;; only starts with an SQL verb is ordinary text.
(defn read-rows [query!]
  (query! "SELECT id FROM direct_rows"))

(defn directory-error [dir]
  (format "create %s dir" dir))

;; A statement whose table format fills in is still a statement; its table is
;; known only at run time.
(defn drop-table-statement [table]
  (format "DROP TABLE IF EXISTS %s" table))

;; Each threaded or nested form on one line sits at its own opening
;; parenthesis, so the two identical replaces stay two calls.
(defn chained-paths [path]
  (-> path (str/replace "/" "-") (str/replace "/" "-")))

(defn nested-paths [path]
  (str/replace (str/replace path "/" "-") "/" "-"))

(defn fail! [reason]
  (throw (ex-info reason {})))

;; A call written in a macro's argument keeps its own place and caller.
(defmacro ensure! [condition]
  `(when-not ~condition (fail! "no limit")))

(defn ensured-limit []
  (ensure! (read-limit)))

;; A forward declaration and its definition repeat one name: the map of parts
;; reads them as one unit.
(declare shout)

(defn loud-greeting [name]
  (shout (service/greet name)))

(defn shout
  "Upper-cases a line. A docstring, like a comment or a blank line, is no
  line of code."
  [line]
  ;; Only the text changes.
  (let [upper (str/upper-case line)]

    (str upper "!")))

;; Mirrors the pandas store-target idiom: an fn inside a set! target or a binding default.
(defn handled [row]
  (.-handled (service/apply-handler (fn [value] (service/greet value)) row)))

(defn mark-handled! [row]
  (set! (.-handled (service/apply-handler (fn [value] (service/greet value)) row)) true))

(defn handled-or-default [row]
  (let [{:keys [handled] :or {handled (service/apply-handler (fn [value] (service/greet value)) row)}} row]
    handled))

;; Mirrors Python's lambda in a function header: FastAPI Depends(lambda: ...) and a lambda default.
(defn handled-param
  [row & {:keys [handled] :or {handled (service/apply-handler (fn [value] (service/greet value)) row)}}]
  handled)

(defn checked-handled [row]
  {:pre [(service/apply-handler (fn [value] (service/greet value)) row)]}
  row)

(def handled-by-default
  (fn [row & {:keys [handled] :or {handled (service/apply-handler (fn [value] (service/greet value)) row)}}]
    handled))

;; A var's metadata and a defn's attr-maps are evaluated once, when the
;; namespace loads, so a call written there belongs to the namespace, as a call
;; in a Python decorator's arguments belongs to the defining scope. A :pre
;; condition or an :or default runs on each call and stays the function's.
(defn ^{:route (service/greet "meta")} routed-by-meta [row]
  (service/greet row))

(defn routed-by-attr-map
  "Greets a row."
  {:route (service/greet "attr")}
  ([row] (service/greet row))
  ([row suffix] (str (service/greet row) suffix))
  {:tail (service/greet "tail")})

(def ^{:route (service/greet "def")} routed-value "value")

(defonce ^{:route (service/greet "once")} routed-once "value")

(defmulti ^{:route (service/greet "multi")} routed-multi :kind)

;; A constructor of an imported class written inside a syntax-quote is
;; reported as a call that names no method. It is no static method call and
;; names no outside function; a static method the source names is one.
(defmacro fresh-list [] `(ArrayList.))

(defn new-id [] (str (java.util.UUID/randomUUID)))

;; Calling an anonymous function literal's argument calls a local clj-kondo
;; gives no name; the call keeps `%` as written.
(defn apply-each [fs] (map #(% 1) fs))

;; One statement handed twice to one call, apart only in spacing, is one
;; statement at that call.
(defn zero-rows [] (str "SELECT 0 AS a" " UNION ALL" " SELECT 0 AS a"))

;; example.facade refers every var of example.rates and then defines its own
;; to-text: the alias reaches the facade's var.
(defn facade-text [day] (facade/to-text day))

;; A private function only cheer calls: the map of parts places it with its
;; one user.
(defn- exclaim [s] (str s "!"))

(defn cheer [name] (exclaim name))

;; A call written in a def's value runs when the namespace loads; the var is
;; its caller, as a Go package-level variable is the caller of its
;; initializer's calls.
(def default-greeting (service/greet "default"))

;; Starts another program, git: the words of its command line are git's.
(defn revision []
  (:out (shell/sh "git" "rev-parse" "HEAD")))
