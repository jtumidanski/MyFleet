import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { AppProviders } from './components/providers/AppProviders';
import { App } from './App';
import { loadRuntimeConfig } from './lib/config/runtimeConfig';
import './index.css';

const rootElement = document.getElementById('root');
if (!rootElement) throw new Error('Root element #root not found');

// Render FIRST, latch the config after. Blocking the mount on the config fetch
// meant a wedged /config/config.json showed a blank page for the full 2 s
// timeout on every route — for a value whose only consumer is one optional icon
// button. Nothing on first paint needs it.
//
// The config is not lost by rendering early: the module is an observable store
// and `useRuntimeConfig` subscribes to it, so components that rendered against
// the compiled-in defaults re-render when the real document arrives. Without
// that subscription this ordering would be the worse bug — a ConfigMap override
// that silently never takes effect.
//
// `void` is safe because loadRuntimeConfig catches everything internally and
// always resolves; there is no rejection to handle.
createRoot(rootElement).render(
  <StrictMode>
    <AppProviders>
      <App />
    </AppProviders>
  </StrictMode>,
);

void loadRuntimeConfig();
