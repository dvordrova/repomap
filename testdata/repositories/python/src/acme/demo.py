"""An incidental demonstration guard inside the declared package."""
def greeting(name):
    return "Hello " + name

if __name__ == "__main__":
    print(greeting("example"))
