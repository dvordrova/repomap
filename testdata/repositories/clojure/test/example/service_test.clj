(ns example.service-test
  (:require [clojure.test :refer [deftest is]]
            [example.service :as service]))

(deftest greeting
  (is (= "HELLO WORLD" (service/greet "world"))))

(deftest reader-forms
  (let [arguments (service/reader-arguments vector "metadata")]
    (is (= 5 (count arguments)))
    (is (= "kept" (first arguments)))
    (is (= "metadata" (nth arguments 3)))))
