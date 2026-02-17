#include <iostream>
#include <vector>
#include <string>

class Logger {
public:
    void log(const std::string& message) {
        entries_.push_back(message);
        std::cout << "[LOG] " << message << std::endl;
    }

    size_t count() const {
        return entries_.size();
    }

private:
    std::vector<std::string> entries_;
};

int main() {
    Logger logger;
    logger.log("Starting application");
    logger.log("Processing data");
    logger.log("Done");
    std::cout << "Total entries: " << logger.count() << std::endl;
    return 0;
}
