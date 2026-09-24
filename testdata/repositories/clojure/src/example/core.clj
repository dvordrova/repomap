(ns example.core
  (:require [example.service :as service]))

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
