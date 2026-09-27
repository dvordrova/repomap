(ns example.facade
  (:require [example.rates :refer :all]))

;; Defined here after referring every public var of example.rates.
(defn to-text [day] (str "day " (index-of day)))
