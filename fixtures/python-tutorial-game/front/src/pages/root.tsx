import React, { FC, useState, useEffect } from "react";
import { Link } from "react-router-dom";
import { Menu, Layout } from "antd";
import styled from "styled-components";

import { getLevels } from "../service/http";

const { Header, Footer, Content } = Layout;

interface RootpageProps {
  children: React.ReactNode;
  levelId?: number;
}

const StyledHeader = styled(Header)`
  padding-inline: 0px;
`;

export const Rootpage: FC<RootpageProps> = (props) => {
  const [menuItems, setMenuItems] = useState<JSX.Element[]>([]);

  useEffect(() => {
    getLevels().then((levels) => {
      let newMenuItems: JSX.Element[] = [];
      for (let i = 0; i < levels.count; ++i) {
        newMenuItems.push(
          <Menu.Item key={i}>
            <Link to={`/level/${i}`}>{i}</Link>
          </Menu.Item>,
        );
      }
      setMenuItems(newMenuItems);
    });
  }, []);

  return (
    <Layout>
      <StyledHeader>
        {props.levelId !== undefined && (
          <Menu mode="horizontal" selectedKeys={[props.levelId.toString()]}>
            <Menu.SubMenu key="SubMenu" title="Уровни">
              {menuItems}
            </Menu.SubMenu>
          </Menu>
        )}
        {props.levelId === undefined && (
          <Menu mode="horizontal">
            <Menu.SubMenu key="SubMenu" title="Уровни">
              {menuItems}
            </Menu.SubMenu>
          </Menu>
        )}
      </StyledHeader>
      <Content>{props.children}</Content>
      <Footer> ©{new Date().getFullYear()} Created by Dmitry Bozhko</Footer>
    </Layout>
  );
};
