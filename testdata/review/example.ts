interface Repository<T> {
  findById(id: string): Promise<T | null>;
  findAll(): Promise<T[]>;
  save(entity: T): Promise<T>;
  delete(id: string): Promise<boolean>;
}

interface User {
  id: string;
  name: string;
  email: string;
  createdAt: Date;
}

class InMemoryUserRepository implements Repository<User> {
  private users: Map<string, User> = new Map();

  async findById(id: string): Promise<User | null> {
    return this.users.get(id) ?? null;
  }

  async findAll(): Promise<User[]> {
    return Array.from(this.users.values());
  }

  async save(user: User): Promise<User> {
    this.users.set(user.id, { ...user });
    return user;
  }

  async delete(id: string): Promise<boolean> {
    return this.users.delete(id);
  }
}

async function main(): Promise<void> {
  const repo = new InMemoryUserRepository();

  await repo.save({
    id: "1",
    name: "Alice",
    email: "alice@example.com",
    createdAt: new Date(),
  });

  await repo.save({
    id: "2",
    name: "Bob",
    email: "bob@example.com",
    createdAt: new Date(),
  });

  const all = await repo.findAll();
  console.log(`Users: ${all.length}`);

  const alice = await repo.findById("1");
  if (alice) {
    console.log(`Found: ${alice.name}`);
  }
}

main().catch(console.error);
