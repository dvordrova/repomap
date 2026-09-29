(ns example.fixtures)

;; Rows the tests share. This namespace requires no test framework: the :test
;; alias runs its directory as tests.
(defn sample-row [] {:name "default"})
