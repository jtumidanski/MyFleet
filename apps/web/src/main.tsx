import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { AppProviders } from './components/providers/AppProviders';
import { App } from './App';
import { loadRuntimeConfig } from './lib/config/runtimeConfig';
import './index.css';

const rootElement = document.getElementById('root');
if (!rootElement) throw new Error('Root element #root not found');

// Runtime config is latched before the tree mounts so synchronous readers
// (getRuntimeConfig) never observe a half-initialised value.
//
// `.then` rather than `.catch` is the mechanism on purpose: loadRuntimeConfig
// catches everything internally and always resolves, so this callback always
// runs and the app always renders — with the compiled-in defaults if the fetch
// failed. It costs one same-origin request ahead of first paint, which is not a
// new class of delay: the app already gates its first meaningful render on the
// auth bootstrap.
void loadRuntimeConfig().then(() => {
  createRoot(rootElement).render(
    <StrictMode>
      <AppProviders>
        <App />
      </AppProviders>
    </StrictMode>,
  );
});
