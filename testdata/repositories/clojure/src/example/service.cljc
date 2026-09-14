(ns example.service
  (:require [clojure.string :as str]))

(defn greet
  "Build the greeting before delivery."
  [name]
  (str/upper-case (str "Hello " name)))

(defn deliver! [destination message]
  (spit destination message))

(defn apply-handler [handler value]
  ;; The local parameter is callable; its runtime target is not a global var.
  (handler value))

(defn platform []
  #?(:clj #_{:clj-kondo/ignore [:unresolved-namespace]}
     (System/currentTimeMillis)
     :cljs (js/Date.now)))

(defn reader-arguments [send! value]
  (send! #_(throw (Exception. "discarded")) "kept"
         #"a+b" '(one two) ^String value #{:one :two}))

;; Imported data is read without calling anything in its defining namespace.
(def source-limit 8)

;; Underscore is still a local binding; its runtime callback is unknown.
(defn underscore-handler [_ value] (_ value))
