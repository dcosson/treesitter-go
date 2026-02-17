interface Shape {
  area(): number;
  perimeter(): number;
}

class Circle implements Shape {
  constructor(private radius: number) {}

  area(): number {
    return Math.PI * this.radius ** 2;
  }

  perimeter(): number {
    return 2 * Math.PI * this.radius;
  }
}

function describeShape(shape: Shape): string {
  return `Area: ${shape.area()}, Perimeter: ${shape.perimeter()}`;
}

const circle = new Circle(5);
console.log(describeShape(circle));
