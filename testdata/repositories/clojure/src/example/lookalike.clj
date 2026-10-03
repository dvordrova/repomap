(ns example.lookalike
  (:refer-clojure :exclude [eval]))

;; This namespace names its own functions like a setting read and code
;; evaluation. A call to them runs this repository's code, whatever the name:
;; it reads no setting and evaluates nothing.
(defn eval [form] form)

(defn getenv [key] (str "database row: " key))

(defn read-lookalikes []
  [(eval "ordinary data") (getenv "CUSTOMER_ROW")])

;; The JVM's own environment read and Clojure's own evaluation.
(defn read-setting []
  (System/getenv "FIXTURE_LOOKALIKE_SETTING"))

(defn evaluate-form [form]
  (clojure.core/eval form))

;; Public declarations whose names hold a dollar sign or start with an
;; underscore are declarations like any other.
(defn price$ [] 42)

(def _token "opaque")
