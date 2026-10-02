// Every card represents one saved report item, including each original input.
export function groupInputs(inputs=[],translate=text=>text) {
  // The kinds as the reading column names its sections (31-reading-column.js
  // rmInputKindTitles): Redis's Inputs had listed three kinds, scheduled
  // and continuous work as one, beside a column of four.
  const kinds=[['request','Incoming requests'],['command','Commands'],['setting','Settings'],['scheduled','Scheduled tasks'],['continuous','Background work'],
    ['interaction','User interactions'],['consumer','Queue consumers'],['extension','Extension points'],['entry','Kind not established']];
  const groups=new Map(kinds.map(([kind,title])=>[kind,{kind,title:translate(title),inputs:[]}]));
  for(const input of inputs){
    const kind=input.activation==='background'?'continuous':input.activation==='queue_consumer'?'consumer':groups.has(input.activation)?input.activation:'entry';
    groups.get(kind).inputs.push(input);
  }
  return [...groups.values()].filter(group=>group.inputs.length);
}

// A title breaks only between words, as the browser breaks it: closing
// punctuation stays with the word before it and an opening bracket with the
// word after it. Redis's input collection read "redis-server (executable"
// above a lone ")". A word breaks inside only when it alone is wider than the
// line: after a separator or between camelCase words when it can, and never
// so that a line starts with closing punctuation or ends with an opening one.
const closing=/^[)\]}»›”’,.;:!?…%]/u,opening=/[(\[{«‹“‘]$/u;
// Whether two names say the same words, whatever their case and spacing.
export const sameWords=(a,b)=>String(a||'').trim().replace(/\s+/g,' ').toLowerCase()===String(b||'').trim().replace(/\s+/g,' ').toLowerCase();
export function titleWords(text){
  const words=[];
  for(const word of String(text||'').split(/\s+/).filter(Boolean)){
    if(words.length&&(closing.test(word)||opening.test(words.at(-1))))words[words.length-1]+=' '+word;
    else words.push(word);
  }
  return words;
}
export function widestWord(text,font,measure){
  return Math.max(0,...titleWords(text).map(word=>measure(word,font)));
}
// The widest piece a title can be broken into: a path breaks after its
// slashes (breakWord), so "build_helpers/
// freqtrade_client_version_align.py" needs its longest piece's width, not
// the whole name's (owner's reviewer, 2026-09-29: it ran into the zoom
// mark and pushed its frame 500 pixels wide).
const separated=/(?<=\/)/u;
export function widestPiece(text,font,measure){
  return Math.max(0,...titleWords(text).flatMap(word=>word.split(separated)).map(piece=>measure(piece,font)));
}
// A path breaks after its slashes first ("scripts/rest_client.py", never
// "scripts/ rest_client. py"); a piece still too wide breaks as any other
// word, keeping every character. Any other word breaks after a separator or
// between camelCase words, never before a file's extension; with no such
// place, where the line ends.
function breakWord(word,width,font,measure){
  if(word.includes('/')){
    const pieces=[''];
    for(const segment of word.split(/(?<=\/)/)){
      const last=pieces.length-1;
      if(measure(pieces[last]+segment,font)<=width){pieces[last]+=segment;continue;}
      pieces.push(segment);
    }
    return pieces.filter(Boolean).flatMap(piece=>measure(piece,font)<=width?[piece]:breakWord(piece.replace(/\//g,'\u2215'),width,font,measure).map(part=>part.replace(/\u2215/g,'/')));
  }
  const letters=Array.from(word),pieces=[];
  const extension=at=>letters[at-1]==='.'&&/^[A-Za-z0-9]{1,5}$/.test(letters.slice(at).join(''));
  for(let start=0;start<letters.length;){
    let end=start+1;
    while(end<letters.length&&measure(letters.slice(start,end+1).join(''),font)<=width)end++;
    if(end<letters.length){
      let found=false;
      for(let at=end;at>start;at--){
        const before=letters[at-1],after=letters[at];
        if((/[_\-.:]/.test(before)&&!extension(at))||/[\p{Ll}\d]/u.test(before)&&/\p{Lu}/u.test(after)||after==='('){end=at;found=true;break;}
      }
      // A word with no such place keeps every character, broken where the
      // line ends: an input's literal is its identity.
      if(!found&&end>start+1&&extension(end))end--;
      while(end>start+1&&(closing.test(letters[end])||opening.test(letters[end-1])))end--;
    }
    pieces.push(letters.slice(start,end).join(''));start=end;
  }
  return pieces;
}
export function wrapText(text,width,font,measure){
    const lines=[''];
    for(const word of titleWords(text)){
      const last=lines.length-1, candidate=lines[last]?`${lines[last]} ${word}`:word;
      if(measure(candidate,font)<=width){lines[last]=candidate;continue;}
      if(lines[last])lines.push('');
      const pieces=measure(word,font)<=width?[word]:breakWord(word,width,font,measure);
      lines[lines.length-1]=pieces[0];lines.push(...pieces.slice(1));
    }
    return lines;
}

// A title reads whole. When its widest word is wider than the room, its type
// shrinks until that word fits rather than breaking it: a destination frame
// fitted one pixel narrower than reserved read "DNS resolve" over a clipped
// "r".
export function fitTitle(title,width,font,measure){
  const widest=widestPiece(title,font,measure),scale=widest>width?width/widest:1;
  return {scale,lines:wrapText(title,scale<1?widest:width,font,measure)};
}

// A program's Outside frame holds its destinations as chips of one size,
// each naming its destination in at most two lines, in rows wrapped toward
// a square instead of one strip (overview.mjs outsideChips): beside or
// under its program it takes little of the map's width.
// `top` is the band its title "Outside" stands in.
export const chip={width:112,height:40,gap:8,side:12,top:34};
// The type size, 12px down to 9px, at which a chip's name stands in its
// two lines of its text column (canvas.css: 8px padding, 1.25px border),
// whole words to a line.
export function chipFont(name,measure){
  const room=chip.width-2*8-2*1.25-1;
  for(let size=12;size>9;size-=.5)if(wrapText(name,room,`600 ${size}px system-ui`,measure).length<=2&&widestWord(name,`600 ${size}px system-ui`,measure)<=room)return size;
  return 9;
}
export function chipGrid(count){
  let best=null;
  for(let columns=1;columns<=Math.max(1,count);columns++){
    const rows=Math.ceil(count/columns);
    const width=2*chip.side+columns*chip.width+(columns-1)*chip.gap,height=chip.top+chip.side+rows*chip.height+(rows-1)*chip.gap;
    const distance=Math.abs(Math.log(width/height));
    if(!best||distance<best.distance-1e-9)best={columns,rows,width,height,distance};
  }
  return best;
}

// A card as canvas.css draws it: 260px wide inside a 1.5px border and 16px
// padding, its title keeping 34px beside it for a part's zoom button. Text is
// wrapped at that real column. Wrapped at 228 and 194, a line between the
// column and those widths broke again in the browser: "Implements Redis set
// commands and" (225.84px in a 225px column) left "and" alone on a line.
export const card={width:260,border:1.5,padding:16,zoom:34};
// The room the note of targets not analysed keeps at the whole map, over
// its words' own size.
const noteRoom=1.5;
export const cardText=card.width-2*card.border-2*card.padding;
// An input's kind mark before its name: 14px, a 6px gap (canvas.css) and
// the browser's rounding.
export const kindMark=22;

// A narrow external heading may need the full card width below its zoom mark.
// Measure the same heading for layout reservation and for the visible card.
export function overviewHeading(item,screenWidth,measure){
  const collection=['communication','inputs'].includes(item.branch);
  const size=collection?13:18,lineHeight=collection?17:23,font=`700 ${size}px system-ui`;
  const title=item.heading||item.name||item.title;
  const clearZoom=widestPiece(title,font,measure)>screenWidth-64;
  const width=Math.max(1,Math.min(304,screenWidth-(clearZoom?(collection?16:32):64)));
  const {scale,lines}=fitTitle(title,width,font,measure);
  return {width,clearZoom,lines,scale,fontSize:size*scale,lineHeight:lineHeight*scale,height:lines.length*lineHeight*scale+(clearZoom?32:0)};
}

// A whole-map fit that cannot give a summary its reserved room (a short window,
// a crowded map) draws the summary at that room and scales it down whole, the
// way a small group keeps a smaller complete label. Drawn into the short box,
// Redis's 1280×720 first screen cut "TCP endpoint" below its frame and
// "Background" out of its input list. The result is the largest scale at
// which the summary's minimum width and measured height fit its screen box.
export function overviewScale(item,screenWidth,screenHeight,availableHeight=Infinity){
  const fits=scale=>screenWidth/scale+1e-6>=(item.overviewMinWidth||0)&&
    screenHeight/scale+1e-6>=(item.overviewHeightAtWidth?.(screenWidth/scale,{availableHeight:availableHeight/scale})||0);
  if(fits(1))return 1;
  let low=0,high=1;
  for(let i=0;i<24;i++){const middle=(low+high)/2;if(fits(middle))low=middle;else high=middle;}
  return Math.max(low,1e-3);
}

// A group's fixed world box can be much smaller than its siblings at the
// common reveal threshold. Fit its complete name once, never hide it or
// rewrap it against the current viewport. The frame itself is painted by Area.
export function groupHeading(node,title,maxScale,measure,reservedWidth=44,reservedHeight=12,minHeight=0){
  const font='600 12px system-ui',widest=widestWord(title,font,measure);
  const lines=scale=>wrapText(title,node.width/scale-reservedWidth,font,measure);
  const fits=scale=>node.height/scale>=minHeight&&node.width/scale-reservedWidth>=widest&&node.height/scale-reservedHeight>=Math.max(20,lines(scale).length*16);
  let scale=maxScale;
  if(!fits(scale)){
    let low=0,high=scale;
    for(let i=0;i<20;i++){
      const middle=(low+high)/2;
      if(fits(middle))low=middle;else high=middle;
    }
    scale=low;
  }
  return {scale,title:lines(scale).join('\n')};
}

// The whole lines of a closed card's description that fit `most` lines of
// `width`: the last one shown ends after a whole word with "…", never
// inside a word (-webkit-line-clamp had cut "applies pluggabl…").
const descriptionFont='11px system-ui';
export function descriptionLines(text,width,most,measure,font=descriptionFont){
  if(!text||most<=0)return [];
  const lines=wrapText(text,width,font,measure);
  if(lines.length<=most)return lines;
  const words=lines[most-1].split(' ');
  while(words.length>1&&measure(`${words.join(' ')}…`,font)>width)words.pop();
  return [...lines.slice(0,most-1),`${words.join(' ')}…`];
}

// A closed card's heading with the whole lines of its description under
// it. The title is fitted as groupHeading fits it; when that leaves the
// description cut, the heading is drawn up to two fifths smaller if that
// lets it read whole, or at least two lines of it (reviewer, 2026-09-30:
// freqtrade's "Core / trading / runtime" had stood in three big lines over
// "Executes trading strategies and…" and an empty strip). `card` is the card's room in its own units:
// `side`, the width the description does not take, and `lines(height,
// titleLines)`, how many 15px lines fit under the title; `least`, the
// smallest scale its title may be drawn at; `grow`, a card of words alone.
export function describedHeading(node,title,text,maxScale,measure,reservedWidth,reservedHeight,minHeight,card){
  const first=groupHeading(node,title,maxScale,measure,reservedWidth,reservedHeight,minHeight);
  const at=scale=>{
    const titleLines=wrapText(title,node.width/scale-reservedWidth,'600 12px system-ui',measure);
    const width=node.width/scale-card.side-1,most=Math.max(0,card.lines(node.height/scale,titleLines.length));
    return {scale,title:titleLines.join('\n'),width,most,whole:!text||wrapText(text,width,descriptionFont,measure).length<=most};
  };
  // Whole if it can be down to 0.6 of the title's size, else at least two
  // lines of it, else as the title fits.
  const ladder=[1,.95,.9,.85,.8,.75,.7,.65,.6].filter(factor=>factor===1||first.scale*factor>=(card.least||0)).map(factor=>at(first.scale*factor));
  let chosen=ladder.find(next=>next.whole)||ladder.find(next=>next.most>=2)||ladder[0];
  // A card that is its words alone (`card.grow`, a loose part) takes the
  // largest size at which its title and its whole description still fit:
  // at the size of the area titles beside it, freqtrade's scripts and
  // litestream's loose parts had stood with their words in the top third of
  // their cards (the reading lints).
  if(card.grow&&chosen.whole){
    const fits=scale=>{
      const next=at(scale),lines=next.title.split('\n');
      return next.whole&&node.height/scale>=minHeight&&node.width/scale-reservedWidth>=Math.max(0,...lines.map(line=>measure(line,'600 12px system-ui')))&&
        node.height/scale-reservedHeight>=Math.max(20,lines.length*16);
    };
    let low=chosen.scale,high=chosen.scale*8;
    for(let i=0;i<24;i++){const middle=(low+high)/2;if(fits(middle))low=middle;else high=middle;}
    chosen=at(low);
  }
  return {scale:chosen.scale,title:chosen.title,lines:descriptionLines(text,chosen.width,chosen.most,measure)};
}

// A callable written inline is named as the reading column names it
// (31-reading-column.js rmInlineText): "anonymous function in
// ReplicateCommand.Run", never GroupsIndex's "ReplicateCommand.Run
// (inline)" on the canvas beside it; "anonymous function in main for
// doctor" for "main (inline for doctor)"; "one of two anonymous functions
// in Start" for "Start (inline, 2)".
export const inlineWritten=/([^\s→(]+) \(inline(?: for ([^)]+)|, (\d+))?\)/g;
const inlineMany={2:'one of two anonymous functions in {0}',3:'one of three anonymous functions in {0}',4:'one of four anonymous functions in {0}',5:'one of five anonymous functions in {0}',6:'one of six anonymous functions in {0}',7:'one of seven anonymous functions in {0}',8:'one of eight anonymous functions in {0}',9:'one of nine anonymous functions in {0}'};
export function inlineName(text,translate=text=>text){
  const said=(key,...values)=>values.reduce((out,value,i)=>out.replace(`{${i}}`,value),translate(key,...values));
  return typeof text==='string'?text.replace(inlineWritten,(_,home,word,count)=>word?said('anonymous function in {0} for {1}',home,word):count?said(inlineMany[count]||'one of many anonymous functions in {0}',home):said('anonymous function in {0}',home)):text;
}

export function prepareCards(records, _inputOwner, measure, translate) {
  const wrap=(text,width,font)=>wrapText(text,width,font,measure);
  records=records.map(n=>n.title&&/ \(inline[ ,)]/.test(n.title)?{...n,title:inlineName(n.title,translate)}:n);
  const byID=new Map(records.map(n=>[n.id,n]));
  // An input collection's inputs, through the part groups inside it.
  const leavesOf=id=>{const n=byID.get(id);return n?.children?.length?n.children.flatMap(leavesOf):[id];};
  const childNames=id=>(byID.get(id)?.children||[]).map(child=>byID.get(child)?.title).filter(Boolean);
  return records.map(n=>{
    const frame=!!n.children?.length;
    // A part's zoom button takes its room beside its title, an input's kind
    // mark its room before it.
    const title=wrap(n.title,!frame&&!n.activation&&n.symbols?.length?cardText-card.zoom:n.activation?cardText-kindMark:cardText,'700 17px system-ui');
    const label=wrap(n.title,152,'12px system-ui');
    const metadata=[n.language,n.componentKind?translate(n.componentKind):''].filter(Boolean).join(' · ');
    const roleLines=n.role?wrap(n.role,cardText,'600 13px system-ui'):[];
    // A part's name alone does not say what it is: its one-sentence
    // description stands under the name, at most three lines. The browser
    // wraps it in the text column; its lines are counted there for the
    // card's height and never inserted into the text.
    const part=!frame&&!n.activation&&!n.branch&&n.category==='part';
    const description=n.branch==='component'||n.category==='component'||part?n.summary||'':'';
    const descriptionMost=part?3:0;
    const descriptionLines=description?Math.min(descriptionMost||Infinity,wrap(description,cardText,'13px system-ui').length):0;
    const subtitle=n.category==='external'&&!n.title.endsWith(n.subtitle||'')?n.subtitle:'';
    const subtitleLines=subtitle?wrap(subtitle,cardText,'13px system-ui'):[];
    // A part named as its program is (a script's one part, named by its
    // file) is not listed again under it (canvas.jsx ComponentOverview).
    const names=n.branch==='component'?childNames(n.id).filter(name=>!sameWords(name,n.title)):[];
    // A program of one or two parts and no area (a script) prefers room
    // for its role and its description, at most three lines: its box takes
    // its summary's proportion (split-layout.mjs widenTo), and freqtrade's
    // scripts had dropped their descriptions.
    const small=n.branch==='component'&&(n.children||[]).length<=2&&(n.children||[]).every(id=>byID.get(id)?.branch!=='area');
    const shownRole=small&&n.role&&!names.some(name=>sameWords(name,n.role))?n.role:'';
    // External headings may use the full width below the zoom mark. Its extra
    // row is reserved by overviewHeading, not by forcing a wider frame.
    const input=!!n.activation,collection=['communication','inputs'].includes(n.branch);
    const inputGroups=n.branch==='inputs'?groupInputs(leavesOf(n.id).map(id=>byID.get(id)).filter(n=>n?.activation),translate):[];
    const widest=(text,font)=>widestWord(text,font,measure);
    // Reserve complete physical pixels. A fractional fit round-off at the
    // longest-word boundary must not unexpectedly add a row below the zoom mark.
    // A plain tile under its display group's heading needs room for its zoom
    // mark alone.
    // An input collection is headed by the glyph key's word: titled with its
    // component's name, Redis's inputs read as a second redis-server.
    const heading=n.branch==='inputs'?translate('Inputs'):n.branch==='outside'?translate('Outside'):'';
    // The Outside frame's summary is its chips: at the whole-map camera
    // they read at their own size, and the frame keeps their proportion.
    const grid=n.branch==='outside'?chipGrid((n.children||[]).length):null;
    const cardHeight=66-(input?24:0)+title.length*22+descriptionLines*18+(subtitleLines.length?8+subtitleLines.length*18:0);
    const unread=!n.branch&&!frame&&n.category==='component';
    // The note's own words at their own size: its title, and the targets'
    // names under it (canvas.jsx Part, a standalone heading).
    const noteWidth=unread?Math.ceil(Math.max(widest(n.title,'600 12px system-ui'),widest(n.summary||'','11px system-ui'))+14):0;
    const unreadNote=unread?{width:noteWidth,title:wrap(n.title,noteWidth-14,'600 12px system-ui'),
      height:12+wrap(n.title,noteWidth-14,'600 12px system-ui').length*16+16+wrap(n.summary||'',noteWidth-14,'11px system-ui').length*15+4}:null;
    const overviewMinWidth=unread?Math.ceil(unreadNote.width*noteRoom):grid?grid.width:Math.ceil(Math.max(widestPiece(heading||n.title,collection?'700 13px system-ui':'700 18px system-ui',measure)+(collection?16:64),
      ...names.map(name=>widest(name,'500 13px system-ui')+32),
      ...inputGroups.map(group=>widest(group.title,'500 13px system-ui')+32+kindMark)));
    // A short target name must not squeeze its area inventory into a column
    // of isolated words. Prefer two-line entries within the existing text
    // column; targets with short area names need no extra width.
    const twoLineWidth=text=>{
      const words=String(text).split(/\s+/).filter(Boolean);
      if(!words.length)return 0;
      return Math.min(...words.map((_,i)=>Math.max(
        measure(words.slice(0,i+1).join(' '),'500 13px system-ui'),
        measure(words.slice(i+1).join(' '),'500 13px system-ui'))));
    };
    const overviewPreferredWidth=grid?grid.width:Math.ceil(Math.max(overviewMinWidth,
      ...names.map(name=>32+Math.min(304,twoLineWidth(name)))));
    // The targets the run could not read are one note, "Not analysed",
    // naming them (page_system_map.go): at the whole map it stands among
    // the programs' summaries with half again their room, its words read
    // as their headings do (canvas.jsx standaloneHeadings), not as a pale
    // card read at four pixels.
    const overviewHeightAtWidth=unread?width=>width*unreadNote.height/unreadNote.width:grid?width=>width*grid.height/grid.width:['component','communication','inputs'].includes(n.branch)?(width,{availableHeight=Infinity,preferred=false}={})=>{
      const heading=overviewHeading(n,width,measure);
      // As the summary draws it (canvas.jsx ComponentOverview): inset 8px in
      // its frame and padded 8px, its heading, a 10px gap, and its kinds 6px
      // apart, each wrapped beside its kind mark. Counted 16px narrower
      // and without the gap, "Commands" had stood cut in half under Inputs.
      if(n.branch==='inputs'){
        const room=Math.max(1,Math.min(304,width-32)-kindMark),kinds=inputGroups.map(group=>wrap(group.title,room,'500 13px system-ui').length*18);
        return Math.max(44,33+Math.max(32,heading.height)+(kinds.length?10+kinds.reduce((a,b)=>a+b,0)+6*(kinds.length-1):0));
      }
      // The inventory remains complete in the scrollable summary. Its first
      // entrance sets the minimum usable height; fitting the entire list would
      // enlarge the world and make its fitted text smaller again.
      const rows=names.map(name=>10+wrap(name,Math.max(1,Math.min(304,width-32)),'500 13px system-ui').length*18);
      const text=Math.max(1,Math.min(304,width-32));
      // Its preferred room only: the fit's minimum stays its heading and list.
      const roleRows=preferred&&shownRole?wrap(shownRole,text,'600 13px system-ui').length*18+10:0;
      const descriptionRows=preferred&&small&&description?10+Math.min(3,wrap(description,text,'13px system-ui').length)*18:0;
      const base=(n.branch==='communication'?16:32)+heading.height+roleRows+descriptionRows;
      const list=rows.length?17+rows.reduce((sum,row)=>sum+row,0):0;
      const controlHeight=28+(n.branch==='communication'?16:32);
      return Math.max(controlHeight,base+(base+list>availableHeight&&rows.length?17+rows[0]:list));
    }:undefined;
    return {...n,heading,note:unreadNote,category:input?'input':n.category,name:n.title,title:title.join('\n'),labelTitle:label.join('\n'),inputGroups,metadata,role:roleLines.join('\n'),overviewHeightAtWidth,overviewMinWidth,overviewPreferredWidth,
      roleLabel:!input&&['core','triggers'].includes(n.lane)?translate(n.lane==='core'?'Core':'Entrypoints'):'',
      description,descriptionMost,subtitle:subtitleLines.join('\n'),
      labelWidth:180,labelHeight:label.length*16,
      headerHeight:Math.max(64,32+title.length*22+(metadata?24:0)+(roleLines.length?8+roleLines.length*18:0)+(descriptionLines?12+descriptionLines*18:0)),
      // An input says its kind by the mark before its name, in no row of its
      // own. A chip is the Outside frame's one size.
      width:n.branch==='chip'?chip.width:card.width,height:n.branch==='chip'?chip.height:frame?undefined:cardHeight,
      // A chip's name reads whole in its two lines, its type smaller when
      // it must: clamped, freqtrade's "External Message Producer" had read
      // "External Message…" (the reading lints).
      chipFont:n.branch==='chip'?chipFont(n.name||n.title,measure):undefined};
  });
}
