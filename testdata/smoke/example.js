function fibonacci(n) {
  if (n <= 1) return n;
  return fibonacci(n - 1) + fibonacci(n - 2);
}

const results = [];
for (let i = 0; i < 10; i++) {
  results.push(fibonacci(i));
}

console.log("Fibonacci:", results.join(", "));

const greet = (name) => {
  return `Hello, ${name}!`;
};

console.log(greet("world"));
