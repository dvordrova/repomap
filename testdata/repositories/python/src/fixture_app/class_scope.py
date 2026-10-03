"""A class body's names are not visible to its methods: Python looks a bare
name used in a method up in the method, the functions around it, the module
and then the builtins, never in the class body. run's eval is the builtin
eval and its range the builtin range, not the class's own methods."""


class Evaluator:
    def eval(self, source):
        return source

    def range(self):
        return [1, 2]

    def run(self, source):
        for _ in range(2):
            eval(source)
        return self.eval(source)
