local function create_counter(initial)
  local count = initial or 0
  return {
    increment = function()
      count = count + 1
    end,
    decrement = function()
      count = count - 1
    end,
    get = function()
      return count
    end,
  }
end

local counter = create_counter(0)
for i = 1, 10 do
  counter.increment()
end
print("Count: " .. counter.get())
