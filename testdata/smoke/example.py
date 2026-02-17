class Calculator:
    def __init__(self):
        self.result = 0

    def add(self, x):
        self.result += x
        return self

    def subtract(self, x):
        self.result -= x
        return self

    def get_result(self):
        return self.result


def main():
    calc = Calculator()
    calc.add(10).subtract(3)
    print(f"Result: {calc.get_result()}")


if __name__ == "__main__":
    main()
