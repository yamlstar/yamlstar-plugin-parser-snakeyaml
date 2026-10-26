#ifndef YAMLSTAR_PLUGIN_H
#define YAMLSTAR_PLUGIN_H

#include <stddef.h>
#include <stdint.h>

#if defined(__GNUC__)
#define YAMLSTAR_PLUGIN_API __attribute__((visibility("default")))
#else
#define YAMLSTAR_PLUGIN_API
#endif

#ifdef __cplusplus
extern "C" {
#endif

YAMLSTAR_PLUGIN_API uint64_t yamlstar_plugin_v2_abi(void);

YAMLSTAR_PLUGIN_API int32_t yamlstar_plugin_v2_manifest(
    uint8_t **output,
    size_t *output_length);

YAMLSTAR_PLUGIN_API void yamlstar_plugin_v2_free(uint8_t *output);

/* EDN event vector on success, UTF-8 error text on failure. */
YAMLSTAR_PLUGIN_API int32_t yamlstar_plugin_v2_transform(
    const uint8_t *input, size_t input_length,
    const uint8_t *options_edn, size_t options_length,
    uint8_t **output, size_t *output_length);

#ifdef __cplusplus
}
#endif

#endif
