struct Counter {
    value: i32,
}

impl Counter {
    fn new() -> Self {
        Counter { value: 0 }
    }

    fn increment(&mut self) {
        self.value += 1;
    }

    fn get(&self) -> i32 {
        self.value
    }
}

fn main() {
    let mut counter = Counter::new();
    for _ in 0..10 {
        counter.increment();
    }
    println!("Count: {}", counter.get());
}
