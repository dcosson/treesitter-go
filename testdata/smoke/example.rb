class Stack
  def initialize
    @data = []
  end

  def push(value)
    @data.push(value)
    self
  end

  def pop
    @data.pop
  end

  def peek
    @data.last
  end

  def size
    @data.length
  end
end

stack = Stack.new
stack.push(1).push(2).push(3)
puts "Top: #{stack.peek}"
puts "Size: #{stack.size}"
