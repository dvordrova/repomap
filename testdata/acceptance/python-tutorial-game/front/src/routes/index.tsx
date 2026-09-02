import React, { lazy, FC } from "react";
import { Routes, Route, useParams } from "react-router-dom";

const HomePage = lazy(() => import("../pages/home"));
const LevelPage = lazy(() => import("../pages/level"));
// const HomePage = lazy(
//   () =>
//     retryImport("../pages/home") as Promise<{ default: ComponentType<any> }>,
// );
// const LevelPage = lazy(
//   () =>
//     retryImport("../pages/level") as Promise<{ default: ComponentType<any> }>,
// );

const AppRoutes: FC = () => {
  return (
    <Routes>
      <Route path="/" element={<HomePage />} />
      <Route
        path="/level/:levelIdFromPath"
        element={<LevelPage key={useParams().levelIdFromPath} />}
      />
      {/* TODO: add next pages
        <Route path="/help" element={<HelpPage />} />
        <Route path="/login" element={<LoginPage />} />
         */}
    </Routes>
  );
};

export { AppRoutes };
