import React from 'react';
import { createRoot } from 'react-dom/client';

import { Store } from '~/store';
import { RouteHistorySourceKind } from '~/store/stores/route';
import { Application, ApplicationProvider } from '~/application';
import { DataLayer } from '~/data-layer/data-layer';
import { UILayer } from '~/ui-layer/ui-layer';
import { Router } from '~/router/router';
import { RouterProvider } from '~/router/Provider';
import { Environment } from '~/environment';

import { FeatureFlags } from '~/domain/features';
import { getAuthToken, getAuthClaims } from '~/utils/cookie';
import { Projects } from '~/utils/projects';

import './blueprint.scss';
import './index.scss';

declare global {
  interface Window {
    debugTools: any;
  }
}

const getApiBaseUrl = (): string => {
  const apiPath = process.env.API_PATH ?? '/api';
  if (process.env.API_SCHEMA && process.env.API_HOST && process.env.API_PORT) {
    return `${process.env.API_SCHEMA}://${process.env.API_HOST}:${process.env.API_PORT}${apiPath}`;
  }
  return `${window.location.origin}${apiPath}`;
};

const run = async () => {
  const authUrl = `${window.location.origin}/api/`;
  const token = getAuthToken();

  if (token === null) {
    window.location.replace(authUrl);
    return;
  }

  const jwtPayload = getAuthClaims(token);
  if (jwtPayload === null || (jwtPayload.exp !== 0 && jwtPayload.exp < Date.now() / 1000)) {
    window.location.replace(authUrl);
    return;
  }

  try {
    await Projects.getInstance().setProjects(token);
  } catch (err) {
    console.error('Failed to fetch projects:', err);
    window.location.replace(authUrl);
    return;
  }

  const store = new Store({ historySource: RouteHistorySourceKind.URL });
  const env = Environment.new();

  const dataLayer = DataLayer.new({
    store,
    customProtocolBaseURL: getApiBaseUrl(),
    customProtocolRequestTimeout: 30000,
    customProtocolMessagesInJSON: false,
    customProtocolCORSEnabled: false,
  });

  const router = new Router(dataLayer);

  const uiLayer = UILayer.new({
    router,
    store,
    dataLayer,
    isCSSVarsInjectionEnabled: true,
  });

  const container = document.getElementById('app');
  if (!container) throw new Error('Expect #app in DOM');
  const root = createRoot(container);

  const renderFn = (_elem: Element, app: Application) => {
    root.render(
      <ApplicationProvider app={app}>
        <RouterProvider router={router} />
      </ApplicationProvider>,
    );
  };

  const app = new Application(env, router, store, dataLayer, uiLayer, renderFn);
  app.mount(container);

  // Trigger feature flags so UILayer.setupEverything() can proceed
  dataLayer.setFeatureFlags(FeatureFlags.default());
};

run();
