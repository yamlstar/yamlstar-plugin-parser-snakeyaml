package org.yamlstar.snakeyaml;

import com.oracle.svm.core.Uninterruptible;
import com.oracle.svm.core.c.function.CEntryPointOptions;
import org.graalvm.nativeimage.c.function.CEntryPoint;
import org.graalvm.nativeimage.c.function.CFunction;
import org.graalvm.nativeimage.c.type.CCharPointer;
import org.graalvm.nativeimage.c.type.CCharPointerPointer;
import org.graalvm.word.Pointer;
import org.graalvm.word.UnsignedWord;

/** Export C forwards without entering Java before the shim attaches a thread. */
public final class NativeAbi {
    private NativeAbi() { }

    @CFunction(value = "snakeyaml_shim_abi", transition = CFunction.Transition.NO_TRANSITION)
    private static native UnsignedWord abiNative();
    @CFunction(value = "snakeyaml_shim_manifest", transition = CFunction.Transition.NO_TRANSITION)
    private static native int manifestNative(CCharPointerPointer out, Pointer size);
    @CFunction(value = "snakeyaml_shim_free", transition = CFunction.Transition.NO_TRANSITION)
    private static native void freeNative(CCharPointer out);
    @CFunction(value = "snakeyaml_shim_transform", transition = CFunction.Transition.NO_TRANSITION)
    private static native int transformNative(CCharPointer input, UnsignedWord length,
        CCharPointer options, UnsignedWord optionsLength,
        CCharPointerPointer out, Pointer size);

    @CEntryPoint(name = "yamlstar_plugin_v2_abi")
    @CEntryPointOptions(prologue = CEntryPointOptions.NoPrologue.class,
        epilogue = CEntryPointOptions.NoEpilogue.class)
    @Uninterruptible(reason = "Forward into C without requiring an isolate")
    public static UnsignedWord abi() { return abiNative(); }

    @CEntryPoint(name = "yamlstar_plugin_v2_manifest")
    @CEntryPointOptions(prologue = CEntryPointOptions.NoPrologue.class,
        epilogue = CEntryPointOptions.NoEpilogue.class)
    @Uninterruptible(reason = "Forward into C without requiring an isolate")
    public static int manifest(CCharPointerPointer out, Pointer size) {
        return manifestNative(out, size);
    }

    @CEntryPoint(name = "yamlstar_plugin_v2_free")
    @CEntryPointOptions(prologue = CEntryPointOptions.NoPrologue.class,
        epilogue = CEntryPointOptions.NoEpilogue.class)
    @Uninterruptible(reason = "Free C memory without requiring an isolate")
    public static void free(CCharPointer out) { freeNative(out); }

    @CEntryPoint(name = "yamlstar_plugin_v2_transform")
    @CEntryPointOptions(prologue = CEntryPointOptions.NoPrologue.class,
        epilogue = CEntryPointOptions.NoEpilogue.class)
    @Uninterruptible(reason = "The C shim owns plugin isolate attachment")
    public static int transform(CCharPointer input, UnsignedWord length,
            CCharPointer options, UnsignedWord optionsLength,
            CCharPointerPointer out, Pointer size) {
        return transformNative(input, length, options, optionsLength, out, size);
    }
}
