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

;; The keys the sketch's key handler reads: on-key hands the pressed key to
;; command-for, which looks it up here, so n and u are the keys key-pressed
;; takes, each with the command it names.
(def ^:private key->command {:n :new-greeting :u :undo})

(defn command-for [key] (get key->command key))

;; Adjacent forms retain separate documentation owners even on one line.
(defn documented-neighbor "The first function keeps its own description." [] 1) (defn undocumented-neighbor [] 2)

;; Both runtimes declare this same protocol and callable interface method.
;; A method declaration does not reveal a runtime implementation.
(defprotocol Reporter
  (report! [this message] "Deliver a message through the installed reporter."))

(def protocol-placeholder nil)

;; Native constructor declarations are callable; the type header is not.
(defrecord Report [message])
(deftype ReportBox [message])

(defn report-message! [reporter message]
  (report! reporter message))
