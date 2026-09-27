// The cubes are views of the key code already printed in each group. They
// neither extend the analysis graph nor attach group relations to a member.
var repomapMembers = (function () {
  var inventories = new WeakMap();
  function sourceKey(source) { return source.Href || source.Open || (source.NoSource ? (source.Path ? JSON.stringify([source.Path,source.Line||0]) : source.Text) : ''); }
  function items(node) {
    if (inventories.has(node)) return inventories.get(node);
    var result = [], known = new Map();
    // A key type the model explained is listed first from its concept; its
    // row in Code in this part still says it is a key and carries its
    // fields, which the concept does not.
    function add(item) {
      var key = sourceKey(item.source);
      if (!key) return;
      var listed = known.get(key);
      if (listed) {
        if (item.key) listed.key = true;
        if (!listed.fields?.length && item.fields?.length) listed.fields = item.fields;
        return;
      }
      known.set(key, item); result.push(item);
    }
    JSON.parse(node.dataset.concepts || '[]').forEach(add);
    var href = node.getAttribute('href') || '';
    var group = href[0] === '#' && document.getElementById(href.slice(1));
    // Code in this part is every declaration of the part, the model's keys
    // first and marked as keys, as its tiles draw them.
    if (group) group.querySelectorAll('.group-highlights .key-symbol').forEach(function (row) {
      var chip = row.querySelector(':scope>strong>.chip') || row.querySelector(':scope>.chip');
      if (!chip) return;
      var name = chip.cloneNode(true); name.querySelectorAll('.ln').forEach(function (n) { n.remove(); });
      var prose = row.querySelector(':scope>.model [data-display-ref]');
      var anchor = row.querySelector(':scope>.anchor,:scope>.model>.model-sources>.anchor');
      var item = {name:name.textContent.trim(), alias:row.dataset.alias||'', key:row.dataset.key==='true', explanation:prose ? prose.textContent : '', explanation_ref:prose?.dataset.displayRef||'', source:{
        Href:chip.dataset.open ? '' : chip.getAttribute('href'), Open:chip.dataset.open || '', NoSource:chip.dataset.noSource==='true', Path:pathOf(chip, function(){return chip.closest('.key-file')?.querySelector('.path')?.textContent;}), Line:Number(chip.dataset.sourceLine)||0,
        Text:anchor ? anchor.textContent : (chip.closest('.key-file').querySelector('.path').textContent + (chip.querySelector('.ln')?.textContent || ''))
      }};
      item.fields = Array.from(row.querySelectorAll(':scope>.symbol-fields>li')).map(function(field){var fieldChip=field.querySelector('.chip');return fieldChip&&inventoryItem(field,fieldChip,function(){return item.source.Path;});}).filter(Boolean);
      add(item);
    });
    // A part without interpreted highlights still has its original source
    // index. Showing those declarations is useful; inventing an explanation
    // or another intermediate group would not be.
    // A type's fields stay inside its row: they are read with the type, not
    // listed as peers of the part's functions.
    if(group&&!result.length)group.querySelectorAll('.symbol-index > li').forEach(function(row){
      var chip=row.querySelector(':scope>.chip');if(!chip)return;
      var item=inventoryItem(row,chip,function(){return row.closest('.inventory-file')?.querySelector('summary')?.textContent.split(' · ')[0];});
      item.fields=Array.from(row.querySelectorAll(':scope>.symbol-fields>li')).map(function(field){var fieldChip=field.querySelector('.chip');return fieldChip&&inventoryItem(field,fieldChip,function(){return item.source.Path;});}).filter(Boolean);
      add(item);
    });
    inventories.set(node, result); return result;
  }
  function inventoryItem(row,chip,heading){
    var name=chip.cloneNode(true);name.querySelectorAll('.ln').forEach(function(n){n.remove();});
    return {name:name.textContent.trim(),alias:row.dataset.alias||'',explanation:'',source:{Href:chip.dataset.open?'':chip.getAttribute('href'),Open:chip.dataset.open||'',NoSource:chip.dataset.noSource==='true', Path:pathOf(chip, heading), Line:Number(chip.dataset.sourceLine)||0,Text:chip.dataset.sourceText||chip.getAttribute('title')||name.textContent.trim()}};
  }
  function size(node) {
    return {w:360,h:112 + Math.min(items(node).length,5)*66 + (items(node).length>5?32:0)};
  }
  // A member's file: the chip's own attribute, else the file heading it
  // sits under, else its title without the line suffix.
  function pathOf(chip,heading){if(chip.dataset.sourcePath)return chip.dataset.sourcePath;var text=typeof heading==='function'?(chip.closest?heading():''):heading;return (text||'').trim()||(chip.getAttribute('title')||'').replace(/:\d+(:\d+)?$/,'');}
  function label(node,item) {
    return displayName(item)+(items(node).some(function(other){return other!==item&&other.name===item.name;})?' · '+item.source.Text:'');
  }
  function displayName(item){return item.alias&&item.alias!==item.name?item.alias+' · '+item.name:item.name;}
  function sourceLink(source) {
    var link=document.createElement(source.Href||source.Open?'a':'span');link.textContent=source.Text;
    if(source.Href){link.href=source.Href;link.target='_blank';link.rel='noopener';}
    else if(source.Open){link.href='#';link.dataset.open=source.Open;}
    else if(source.NoSource)link.title=rmT('No source');
    return link;
  }
  function grid(map,node,limit) {
    var grid=document.createElement('div');grid.className='map-member-grid';
    var list=items(node).slice(0,limit===undefined?items(node).length:limit);
    // The complete list of a large part reads by file: a heading per source
    // file, its members beneath. The five-cube preview inside the node stays flat.
    var paths=new Set(list.map(function(item){return item.source.Path||'';}));
    var byFile=limit===undefined&&list.length>8&&paths.size>1;
    var lastPath=null;
    list.forEach(function(item){
      if(byFile&&(item.source.Path||'')!==lastPath){lastPath=item.source.Path||'';var head=document.createElement('div');head.className='map-member-file';head.textContent=lastPath||rmT('Other');grid.appendChild(head);}
      var row=document.createElement('div');row.className='map-member'+(item.key?' map-member-key':'');
      var name=sourceLink(item.source);name.className='map-member-name';name.textContent=displayName(item);
      name.dataset.memberSource=sourceKey(item.source);
      name.title=[item.explanation,item.source.Text,item.source.NoSource?rmT('No source'):''].filter(Boolean).join('\n');
      name.addEventListener('click',function(e){e.preventDefault();e.stopPropagation();map.showMember(node,item);});
      if(item.source.NoSource){name.setAttribute('tabindex','0');name.setAttribute('role','button');name.addEventListener('keydown',function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();map.showMember(node,item);}});}
      row.appendChild(name);
      if(item.fields?.length){
        var fields=document.createElement('div');fields.className='map-member-fields';
        item.fields.forEach(function(field,index){if(index)fields.appendChild(document.createTextNode(' · '));var link=sourceLink(field.source);link.textContent=field.name;fields.appendChild(link);});
        row.appendChild(fields);
      }
      grid.appendChild(row);
    });
    return grid;
  }
  function draw(map, node, box) {
    var svg = map.querySelector('svg'), previous = svg.querySelector('.map-member-layer');
    if (previous) previous.remove();
    if (!node || !box || !items(node).length) return;
    var layer = document.createElementNS('http://www.w3.org/2000/svg','foreignObject');
    layer.setAttribute('class','map-member-layer');
    layer.setAttribute('x',box.x + 16); layer.setAttribute('y',box.y + 70);
    layer.setAttribute('width',box.w - 32); layer.setAttribute('height',box.h - 78);
    var body = document.createElement('div'); body.className='map-member-content';
    var heading = document.createElement('div'); heading.className='map-member-label';
    heading.textContent=rmT('Code in this part')+' · '+items(node).length; body.appendChild(heading);
    body.appendChild(grid(map,node,5));
    if(items(node).length>5){
      var more=document.createElement('button');more.type='button';more.className='map-members-all';more.textContent=rmT('All {0} →',items(node).length);
      more.addEventListener('click',function(e){e.stopPropagation();map.showAllMembers(node);});body.appendChild(more);
    }
    layer.appendChild(body); svg.appendChild(layer);
  }
  return {sourceKey:sourceKey,items:items,size:size,draw:draw,grid:grid,label:label,displayName:displayName,sourceLink:sourceLink};
})();
