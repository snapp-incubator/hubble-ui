import React from 'react';
import { createRoot } from 'react-dom/client';

import { Environment } from '~/environment';
import { Store } from '~/store';
import { DataLayer } from '~/data-layer';
import { Router, RouterProvider } from '~/router';
import { UILayer } from '~/ui-layer';
import { Application, ApplicationProvider } from '~/application';

import { getAuthToken, getAuthClaims } from '~/utils/cookie';
import { Projects } from '~/utils/projects';

import './blueprint.scss';
import './index.scss';

declare global {
  interface Window {
    debugTools: any;
  }
}

const buildAPIUrl = (env: Environment): string => {
  if (!env.isDev) {
    const fallbackAPIUrl = `${document.location.origin}/api/`;
    try {
      const base = document.querySelector('base')?.href;
      if (base != null) return new URL('api/', base).href;
    } catch (e) {
      console.error('Failed to determine API path', e);
    }
    return fallbackAPIUrl;
  }

  // NOTE: Do not edit those `process.env.VAR_NAME` variable accesses
  // because they only work if you have such a direct call for them.
  const schemaRaw = process.env.API_SCHEMA || document.location.protocol || 'http';
  const schema = schemaRaw.endsWith(':') ? schemaRaw : `${schemaRaw}:`;
  const hostname = process.env.API_HOST || document.location.hostname;
  const port = process.env.API_PORT || document.location.port;
  const path = process.env.API_PATH || 'api';
  const slashedPath = path?.startsWith('/') ? path : `/${path}`;

  return `${schema}//${hostname}${port ? `:${port}` : ''}${slashedPath}`;
};

// Multi-tenancy: the app is only usable with a valid auth token and a list of
// the user's authorized projects (namespaces). Otherwise redirect to the
// backend auth endpoint that starts the login flow.
const ensureAuthorized = async (): Promise<boolean> => {
  const authUrl = `${document.location.origin}/api/`;
  const token = getAuthToken();

  if (token === null) {
    window.location.replace(authUrl);
    return false;
  }

  const claims = getAuthClaims(token);
  if (claims === null || (claims.exp !== 0 && claims.exp < Date.now() / 1000)) {
    window.location.replace(authUrl);
    return false;
  }

  try {
    await Projects.getInstance().setProjects(token);
  } catch (err) {
    console.error('Failed to fetch authorized projects:', err);
    window.location.replace(authUrl);
    return false;
  }

  return true;
};

const run = async () => {
  const env = Environment.new();

  if (!env.isDev && !(await ensureAuthorized())) return;

  const store = new Store();

  const apiUrl = buildAPIUrl(env);
  const dataLayer = DataLayer.new({
    store,
    customProtocolBaseURL: apiUrl,
    customProtocolRequestTimeout: 3000,
    customProtocolMessagesInJSON: env.isDev,
    customProtocolCORSEnabled: true,
  });

  const router = new Router(dataLayer);

  const uiLayer = UILayer.new({
    router,
    store,
    dataLayer,
    isCSSVarsInjectionEnabled: true,
  });

  const renderFn = (targetElem: Element, app: Application) => {
    const root = createRoot(targetElem);

    // NOTE: Use RouterProvider here not to create dependency cycle:
    // Application -> Router -> <Our app component> -> useApplication
    root.render(
      <ApplicationProvider app={app}>
        <RouterProvider router={app.router} />
      </ApplicationProvider>,
    );
  };

  const app = new Application(env, router, store, dataLayer, uiLayer, renderFn);

  app
    .onBeforeMount(() => {
      uiLayer.onBeforeMount();
    })
    .onMounted(app => {
      app.uiLayer.onMounted();
    })
    .mount('#app');
};

// TODO: run() if only we are running not as library
run();
