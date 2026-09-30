(ns example.web
  (:require [example.service :as service]))

;; The page logs a fresh greeting every second: a JavaScript timer is handed
;; the repository's function.
(defn refresh! []
  (js/console.log (service/greet "browser")))

;; The build's :init-fn: shadow-cljs starts the page here.
(defn init []
  (js/setInterval refresh! 1000))

;; A forward declaration defines nothing in this view either: tick is its
;; defn alone, and schedule's call reaches it.
(declare tick)

(defn schedule [] (tick))

(defn tick [] (service/greet "tick"))
