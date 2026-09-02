import React, { Suspense } from "react";
import { ErrorBoundary } from "react-error-boundary";
import { BrowserRouter } from "react-router-dom";
import { AppRoutes } from "./routes";
import { ErrorPage } from "./pages/error";

import { ConfigProvider, theme } from "antd";

function App() {
  return (
    <ErrorBoundary FallbackComponent={ErrorPage}>
      <BrowserRouter>
        <ConfigProvider theme={{ algorithm: theme.darkAlgorithm }}>
          <Suspense fallback={<div>Loading...</div>}>
            <AppRoutes />
          </Suspense>
        </ConfigProvider>
      </BrowserRouter>
    </ErrorBoundary>
  );
}

export default App;
