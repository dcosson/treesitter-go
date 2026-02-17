#include <stdio.h>
#include <stdlib.h>

typedef struct {
    int *data;
    int size;
    int capacity;
} IntVec;

IntVec *intvec_new(int capacity) {
    IntVec *v = malloc(sizeof(IntVec));
    v->data = malloc(sizeof(int) * capacity);
    v->size = 0;
    v->capacity = capacity;
    return v;
}

void intvec_push(IntVec *v, int value) {
    if (v->size >= v->capacity) {
        v->capacity *= 2;
        v->data = realloc(v->data, sizeof(int) * v->capacity);
    }
    v->data[v->size++] = value;
}

void intvec_free(IntVec *v) {
    free(v->data);
    free(v);
}

int main(void) {
    IntVec *v = intvec_new(4);
    for (int i = 0; i < 10; i++) {
        intvec_push(v, i * i);
    }
    printf("Size: %d\n", v->size);
    intvec_free(v);
    return 0;
}
