<script lang="ts">
  export let variant: 'line'|'rect'|'circle' = 'line';
  export let width: string | number = '100%';
  export let height: string | number = '16px';
  export let className = '';
  $: style = `width:${typeof width === 'number' ? width+'px' : width};height:${typeof height === 'number' ? height+'px' : height};`;
</script>

<div class={`skeleton ${variant} ${className}`} style={style} aria-hidden="true"></div>

<style>
  .skeleton{ background: linear-gradient(90deg, rgba(255,255,255,0.03), rgba(255,255,255,0.06), rgba(255,255,255,0.03));
    border-radius:6px; position:relative; overflow:hidden;
  }
  .skeleton::after{
    content:''; position:absolute; left:-150%; top:0; bottom:0; width:150%;
    background: linear-gradient(90deg, transparent, rgba(255,255,255,0.06), transparent);
    animation: shimmer 1.2s infinite linear;
  }
  @keyframes shimmer { from { transform: translateX(0) } to { transform: translateX(100%) } }

  .skeleton.circle{ border-radius:50%; }
  .skeleton.line{ height:14px; }
  .skeleton.rect{ border-radius:6px; }
</style>
