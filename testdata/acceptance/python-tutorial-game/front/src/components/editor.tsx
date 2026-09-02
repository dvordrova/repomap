import React, { useState, useCallback } from "react";
import AceEditor from "react-ace";

import { Spin } from "antd";
import styled from "styled-components";

import "ace-builds/src-noconflict/mode-python";
import "ace-builds/src-noconflict/theme-github_dark";

interface EdirotProps {
  code: string;
  setCode: (value: string) => void;
}

const StyledEditor = styled(AceEditor)`
  min-height: calc(100vh - 255.5px);
`;

export default function Editor({ code, setCode }: EdirotProps) {
  const [editorLoaded, setEditorLoaded] = useState(false);

  const handleCodeChange = useCallback(
    (value: string, _: any) => {
      setCode(value);
    },
    [setCode],
  );

  return (
    <>
      <Spin tip="Editor loading..." spinning={!editorLoaded}>
        <StyledEditor
          value={code}
          placeholder="Ваш python код здесь"
          mode="python"
          theme="github_dark"
          name="blah2"
          onLoad={() => setEditorLoaded(true)}
          onChange={handleCodeChange}
          fontSize={16}
          showPrintMargin={true}
          showGutter={true}
          highlightActiveLine={true}
          setOptions={{
            enableBasicAutocompletion: true,
            enableLiveAutocompletion: true,
            enableSnippets: false,
            showLineNumbers: true,
            tabSize: 2,
          }}
          wrapEnabled={true}
          width={`100%`}
          height={`calc(100vh - 500px)`}
        />
      </Spin>
    </>
  );
}
