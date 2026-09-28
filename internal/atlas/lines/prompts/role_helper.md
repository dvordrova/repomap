
Each question gives one declaration of the program in `declaration`: a
function, a variable, or a type together with its methods, with its kind,
its name, its signature, its `file` and how many `lines` of code it has.
`calls` lists the declarations of the program it calls and `called_by` the
declarations that call it, each as "path:name"; `read_by` lists the
declarations that read it. `handed_over_by` lists the
declarations that hand it over to be called later, such as a callback given
to an event loop or a row of a command table; `registered` is what the code
wrote there: the words it wrote. The question asks what the declaration is
on our map: a helper, the code of a responsibility, or none of these.
