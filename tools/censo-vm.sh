#!/bin/sh
# Censo de la frontera VM (migracion-vm.md M01): inventario MECÁNICO de cada
# símbolo de gopher-lua que el kernel usa, por fichero. Es el mapa del que
# dependen M05-M13: lo que no salga aquí, no cruza la frontera.
#
# Uso: tools/censo-vm.sh            (resumen: símbolos por frecuencia)
#      tools/censo-vm.sh --files    (desglose por fichero)
#      tools/censo-vm.sh --check    (guardia de CI: falla si aparece un símbolo
#                                    gopher-lua NUEVO no registrado en la
#                                    allowlist de abajo — detecta acoplamiento
#                                    nuevo durante la migración)
set -e
cd "$(dirname "$0")/.."

SRC=$(find internal -name '*.go' ! -name '*_test.go')

symbols() {
  # tipos y constantes (lua.Xxx, sin el prefijo) + métodos sobre el estado Lua
  # (L.Xxx / co.Xxx / ls.Xxx / rt.L.Xxx)
  grep -rho 'lua\.[A-Z][A-Za-z0-9]*' $SRC | sed 's/^lua\.//'
  grep -rhoE '\b(L|co|ls|rt\.L)\.[A-Z][A-Za-z0-9]*' $SRC | sed 's/.*\.//'
}

# allowlist: los símbolos que HOY cruzan la frontera (categorías del censo, ver
# docs/migracion-vm-censo.md). Un símbolo fuera de esta lista es acoplamiento
# nuevo a gopher-lua — se revisa antes de dejarlo entrar durante la migración.
ALLOW="LState LNil LString LValue LTable LNumber LFunction LBool LVAsBool
LNilType LUserData LTrue LFalse LGFunction LTFunction Options ApiError
Upvalue P MultRet NewState NewFunction NewTable NewUserData NewTypeMetatable
NewClosure NewThread Push Pop Get GetTop SetTop Insert SetGlobal GetGlobal
SetField GetField RawGet RawSet RawSetString RawGetString RawSetInt RawGetInt
SetMetatable GetTypeMetatable GetMetatable GetMetaField CheckString CheckInt
CheckNumber CheckBool CheckTable CheckFunction CheckUserData CheckAny OptString
OptTable OptInt Call PCall CallByParam LoadString LoadFile DoString Error
RaiseError ToStringMeta Close Resume Status SetContext OpenBase OpenTable
OpenString OpenMath OpenCoroutine OpenOs OpenPackage LoadLibName BaseLibName
TabLibName StringLibName MathLibName CoroutineLibName OsLibName PackageName
IoLibName DebugLibName RegistryIndex"

case "${1:-}" in
  --files)
    for f in $SRC; do
      n=$(grep -c 'lua\.' "$f" 2>/dev/null || true)
      [ "${n:-0}" -gt 0 ] && printf '%4d  %s\n' "$n" "$f"
    done | sort -rn
    ;;
  --check)
    unknown=$(symbols | sort -u | while read -r s; do
      printf '%s\n' $ALLOW | grep -qxF "$s" || echo "$s"
    done)
    if [ -n "$unknown" ]; then
      echo "SÍMBOLOS gopher-lua NUEVOS fuera de la allowlist del censo (M01):"
      echo "$unknown"
      exit 1
    fi
    echo "censo OK: sin símbolos gopher-lua nuevos fuera de la allowlist"
    ;;
  *)
    symbols | sort | uniq -c | sort -rn
    ;;
esac
