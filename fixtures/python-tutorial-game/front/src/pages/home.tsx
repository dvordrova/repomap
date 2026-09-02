import React from "react";
import styled from "styled-components";

import { Rootpage } from "./root";

const Home = styled.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 20px;
  height: 100vh;
  background-color: #f5f5f5;
`;

function HomePage() {
  return (
    <Rootpage>
      <Home>
        <h1>Home</h1>
      </Home>
    </Rootpage>
  );
}

export default HomePage;
