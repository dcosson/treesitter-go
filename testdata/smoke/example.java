import java.util.ArrayList;
import java.util.List;

public class Example {
    private List<String> items;

    public Example() {
        this.items = new ArrayList<>();
    }

    public void addItem(String item) {
        items.add(item);
    }

    public int getCount() {
        return items.size();
    }

    public static void main(String[] args) {
        Example example = new Example();
        example.addItem("alpha");
        example.addItem("beta");
        System.out.println("Count: " + example.getCount());
    }
}
