(ns example.core
  (:require [example.service :as service] [clojure.string :as str]))

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

;; Mirrors the pandas store-target idiom: an fn inside a set! target or a binding default.
(defn handled [row]
  (.-handled (service/apply-handler (fn [value] (service/greet value)) row)))

(defn mark-handled! [row]
  (set! (.-handled (service/apply-handler (fn [value] (service/greet value)) row)) true))

(defn handled-or-default [row]
  (let [{:keys [handled] :or {handled (service/apply-handler (fn [value] (service/greet value)) row)}} row]
    handled))
