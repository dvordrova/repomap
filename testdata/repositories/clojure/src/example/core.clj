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
