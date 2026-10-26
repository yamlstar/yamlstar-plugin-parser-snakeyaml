#include "yamlstar_plugin.h"
#include <pthread.h>
#include <stdlib.h>
#include <string.h>

/* Private, prefixed Native Image C API. No host isolate pointers cross here. */
typedef struct graal_isolate graal_isolate_t;
typedef struct graal_thread graal_isolatethread_t;
extern int snakeyaml_create_isolate(void *, graal_isolate_t **,
                                  graal_isolatethread_t **);
extern int snakeyaml_attach_thread(graal_isolate_t *, graal_isolatethread_t **);
extern graal_isolatethread_t *snakeyaml_get_current_thread(graal_isolate_t *);
extern int snakeyaml_detach_thread(graal_isolatethread_t *);
extern int snakeyaml_parse(graal_isolatethread_t *, const uint8_t *, size_t,
                          const uint8_t *, size_t, uint8_t **, size_t *);

static pthread_once_t initialize_once = PTHREAD_ONCE_INIT;
static graal_isolate_t *isolate;
static int initialization_status;
static const char manifest[] =
    "{:abi 2 :api \"yaml-parser\" :name \"snakeyaml\" :version \"0.1.0\" "
    ":kind \"event-source\" :event-format \"yamlstar-events-edn-v1\"}";

static void initialize(void) {
    graal_isolatethread_t *thread = NULL;
    initialization_status = snakeyaml_create_isolate(NULL, &isolate, &thread);
    if (initialization_status == 0)
        initialization_status = snakeyaml_detach_thread(thread);
}

static int32_t copy_output(const char *text, int32_t status,
                           uint8_t **output, size_t *length) {
    *length = strlen(text);
    *output = malloc(*length ? *length : 1);
    if (!*output) { *length = 0; return 2; }
    memcpy(*output, text, *length);
    return status;
}

uint64_t snakeyaml_shim_abi(void) { return 2; }

void snakeyaml_shim_free(uint8_t *output) { free(output); }

int32_t snakeyaml_shim_manifest(uint8_t **output, size_t *output_length) {
    if (!output || !output_length) return 2;
    *output = NULL;
    *output_length = 0;
    return copy_output(manifest, 0, output, output_length);
}

int32_t snakeyaml_shim_transform(
        const uint8_t *input, size_t input_length,
        const uint8_t *options, size_t options_length,
        uint8_t **output, size_t *output_length) {
    if (!output || !output_length) return 2;
    *output = NULL;
    *output_length = 0;
    if ((!input && input_length) || (!options && options_length))
        return copy_output("Invalid input buffer", 2, output, output_length);
    if (pthread_once(&initialize_once, initialize) || initialization_status)
        return copy_output("Cannot initialize SnakeYAML isolate", 2,
                           output, output_length);
    graal_isolatethread_t *thread = snakeyaml_get_current_thread(isolate);
    int attached = thread == NULL;
    if (attached && snakeyaml_attach_thread(isolate, &thread))
        return copy_output("Cannot attach SnakeYAML thread", 2,
                           output, output_length);
    int32_t status = snakeyaml_parse(thread, input, input_length,
                                   options, options_length, output, output_length);
    if (attached && snakeyaml_detach_thread(thread)) {
        free(*output);
        *output = NULL;
        *output_length = 0;
        return copy_output("Cannot detach SnakeYAML thread", 2,
                           output, output_length);
    }
    return status;
}
