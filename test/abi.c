#include "yamlstar_plugin.h"
#include <assert.h>
#include <dlfcn.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static int32_t (*transform)(const uint8_t *, size_t, const uint8_t *, size_t,
                            uint8_t **, size_t *);
static void (*release_output)(uint8_t *);

static void parse(const char *input, int valid) {
    uint8_t *output = NULL;
    size_t length = 0;
    int32_t status = transform((const uint8_t *)input, strlen(input),
                              (const uint8_t *)"{}", 2, &output, &length);
    if ((status == 0) != valid || !output || !length) {
        fprintf(stderr, "status %d: %.*s\n", status, (int)length,
                output ? (const char *)output : "");
        abort();
    }
    if (valid) {
        assert(output[0] == '[');
        assert(output[length - 1] == ']');
    }
    release_output(output);
}

static void *worker(void *argument) {
    (void)argument;
    for (int i = 0; i < 100; i++) {
        parse("a: &a [one, 'two', \"three\"]\nb: *a", 1);
        parse("[unterminated", 0);
    }
    return NULL;
}

int snakeyaml_probe(const char *path) {
    void *library = dlopen(path, RTLD_NOW | RTLD_LOCAL);
    if (!library) { fprintf(stderr, "%s\n", dlerror()); return 1; }
    uint64_t (*abi)(void) = (uint64_t (*)(void))dlsym(
        library, "yamlstar_plugin_v2_abi");
    int32_t (*manifest)(uint8_t **, size_t *) =
        (int32_t (*)(uint8_t **, size_t *))dlsym(
            library, "yamlstar_plugin_v2_manifest");
    transform = (int32_t (*)(const uint8_t *, size_t, const uint8_t *, size_t,
                             uint8_t **, size_t *))dlsym(
        library, "yamlstar_plugin_v2_transform");
    release_output = (void (*)(uint8_t *))dlsym(
        library, "yamlstar_plugin_v2_free");
    assert(abi && manifest && transform && release_output && abi() == 2);
    uint8_t *output = NULL;
    size_t length = 0;
    assert(manifest(&output, &length) == 0);
    assert(length > 0);
    release_output(output);
    release_output(NULL);
    assert(manifest(NULL, &length) != 0);
    assert(transform(NULL, 1, NULL, 0, &output, &length) != 0);
    release_output(output);
    const uint8_t invalid_utf8[] = {0xff};
    assert(transform(invalid_utf8, 1, NULL, 0, &output, &length) != 0);
    release_output(output);
    assert(transform((const uint8_t *)"x", 1,
                     (const uint8_t *)"{:unknown true}", 15,
                     &output, &length) != 0);
    release_output(output);
    parse("", 1);
    parse("[🙂, λ]\r\n", 1);
    parse("|\n  line", 1);
    parse("%YAML 1.2\n%TAG !e! tag:example.com,2026:\n--- !e!x value\n...", 1);
    pthread_t threads[8];
    for (int i = 0; i < 8; i++)
        assert(pthread_create(&threads[i], NULL, worker, NULL) == 0);
    for (int i = 0; i < 8; i++) assert(pthread_join(threads[i], NULL) == 0);
    /* The GraalVM library remains loaded for the process lifetime. */
    puts("SnakeYAML ABI 2 test passed");
    return 0;
}

#ifndef SNAKEYAML_GRAAL_HOST
int main(int argc, char **argv) {
    assert(argc == 2);
    return snakeyaml_probe(argv[1]);
}
#endif
